import { useCallback, useMemo, useRef, useState } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  RefreshControl,
  Alert,
} from "react-native";
import { useRouter } from "expo-router";
import { useQueryClient } from "@tanstack/react-query";
import Swipeable from "react-native-gesture-handler/Swipeable";
import * as Haptics from "expo-haptics";
import { colors, tabIcons } from "@/lib/theme";
import { useAppStore } from "@/lib/store";
import {
  useIdeasByPeer,
  useDeleteIdea,
  queryKeys,
} from "@/lib/hooks";
import { PeerTree } from "@/components/PeerTree";
import { StatusBadge } from "@/components/StatusBadge";
import type { Idea, IdeaPriority, IdeaStatus } from "@/lib/types";

// ---------------------------------------------------------------------------
// Filters
// ---------------------------------------------------------------------------

type FilterOption = "all" | IdeaStatus;

const FILTERS: { key: FilterOption; label: string }[] = [
  { key: "all", label: "All" },
  { key: "new", label: "New" },
  { key: "planned", label: "Planned" },
  { key: "in-progress", label: "Active" },
  { key: "done", label: "Done" },
  { key: "rejected", label: "Rejected" },
];

const priorityColors: Record<IdeaPriority, string> = {
  low: colors.text.muted,
  medium: colors.status.warning,
  high: "#f97316",
  critical: colors.status.error,
};

const statusMap: Record<IdeaStatus, "running" | "success" | "warning" | "error" | "idle"> = {
  new: "idle",
  planned: "warning",
  "in-progress": "running",
  done: "success",
  rejected: "error",
};

function formatDate(iso: string): string {
  try {
    const d = new Date(iso);
    const month = d.toLocaleString("en", { month: "short" });
    return `${month} ${d.getDate()}`;
  } catch {
    return "";
  }
}

// ---------------------------------------------------------------------------
// Idea Row (with swipe-to-delete)
// ---------------------------------------------------------------------------

function IdeaRow({
  idea,
  onDelete,
}: {
  idea: Idea;
  onDelete: (id: string) => void;
}) {
  const router = useRouter();
  const swipeableRef = useRef<Swipeable>(null);

  const renderRightActions = () => (
    <Pressable
      className="justify-center px-6"
      style={{ backgroundColor: colors.status.error }}
      onPress={() => {
        swipeableRef.current?.close();
        onDelete(idea.id);
      }}
    >
      <Text className="text-white font-semibold text-sm">Delete</Text>
    </Pressable>
  );

  return (
    <Swipeable
      ref={swipeableRef}
      renderRightActions={renderRightActions}
      rightThreshold={40}
      overshootRight={false}
    >
      <Pressable
        className="py-3 px-4 active:opacity-70"
        style={{ backgroundColor: colors.bg.card }}
        onPress={() => router.push(`/ideas/${idea.id}`)}
      >
        {/* Title line */}
        <View className="flex-row items-center justify-between">
          <Text
            className="text-text-primary text-sm font-medium flex-1 mr-3"
            numberOfLines={1}
          >
            {idea.title}
          </Text>
          <Text className="text-text-muted text-xs">
            {formatDate(idea.created_at)}
          </Text>
        </View>

        {/* Meta line: marker + priority dot + status badge */}
        <View className="flex-row items-center mt-1.5 gap-2">
          <Text
            className="text-text-muted text-xs"
            style={{ fontFamily: "Courier" }}
          >
            {idea.marker}
          </Text>
          <View className="flex-row items-center gap-1">
            <View
              className="w-2 h-2 rounded-full"
              style={{
                backgroundColor:
                  priorityColors[idea.priority] ?? colors.text.muted,
              }}
            />
            <Text
              className="text-xs"
              style={{
                color: priorityColors[idea.priority] ?? colors.text.muted,
              }}
            >
              {idea.priority}
            </Text>
          </View>
          <StatusBadge
            status={statusMap[idea.status]}
            label={idea.status}
          />
        </View>

        {/* Description preview */}
        {idea.description ? (
          <Text
            className="text-text-muted text-xs mt-1.5"
            numberOfLines={1}
          >
            {idea.description}
          </Text>
        ) : null}

        {/* Tags */}
        {idea.tags.length > 0 && (
          <View className="flex-row flex-wrap gap-1.5 mt-2">
            {idea.tags.slice(0, 3).map((tag) => (
              <View
                key={tag}
                className="px-2 py-0.5 rounded-full"
                style={{ backgroundColor: `${colors.accent.default}15` }}
              >
                <Text
                  style={{ color: colors.accent.hover }}
                  className="text-xs"
                >
                  {tag}
                </Text>
              </View>
            ))}
            {idea.tags.length > 3 && (
              <Text className="text-text-muted text-xs self-center">
                +{idea.tags.length - 3}
              </Text>
            )}
          </View>
        )}
      </Pressable>
    </Swipeable>
  );
}

// ---------------------------------------------------------------------------
// Ideas List Screen
// ---------------------------------------------------------------------------

export default function IdeasListScreen() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const connected = useAppStore((s) => s.connected);

  const { data: byPeer, isLoading } = useIdeasByPeer();
  const deleteMutation = useDeleteIdea();

  const [filter, setFilter] = useState<FilterOption>("all");
  const [refreshing, setRefreshing] = useState(false);

  // Filter ideas by status
  const filteredByPeer = useMemo(() => {
    if (!byPeer || filter === "all") return byPeer;
    const result: Record<string, Idea[]> = {};
    for (const [peer, ideas] of Object.entries(byPeer)) {
      const filtered = ideas.filter((i) => i.status === filter);
      if (filtered.length > 0) {
        result[peer] = filtered;
      }
    }
    return result;
  }, [byPeer, filter]);

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    try {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.ideasByPeer,
      });
    } finally {
      setRefreshing(false);
    }
  }, [queryClient]);

  const handleDelete = useCallback(
    (id: string) => {
      const idea = Object.values(byPeer ?? {})
        .flat()
        .find((i) => i.id === id);
      const title = idea?.title ?? "this idea";

      Alert.alert(
        "Delete Idea",
        `Delete "${title}"? It can be restored from Recall.`,
        [
          { text: "Cancel", style: "cancel" },
          {
            text: "Delete",
            style: "destructive",
            onPress: () => {
              Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
              deleteMutation.mutate(id);
            },
          },
        ],
      );
    },
    [byPeer, deleteMutation],
  );

  // Totals
  const totalCount = Object.values(byPeer ?? {}).reduce(
    (sum, arr) => sum + arr.length,
    0,
  );
  const filteredCount = Object.values(filteredByPeer ?? {}).reduce(
    (sum, arr) => sum + arr.length,
    0,
  );

  return (
    <View className="flex-1 bg-bg-primary">
      <ScrollView
        className="flex-1"
        contentContainerStyle={{ paddingBottom: 100 }}
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            onRefresh={onRefresh}
            tintColor={colors.text.muted}
          />
        }
      >
        {/* Status Filter Chips */}
        <ScrollView
          horizontal
          showsHorizontalScrollIndicator={false}
          className="px-4 pt-4 pb-3"
          contentContainerStyle={{ gap: 8 }}
        >
          {FILTERS.map((f) => {
            const active = filter === f.key;
            return (
              <Pressable
                key={f.key}
                onPress={() => setFilter(f.key)}
                className="px-3.5 py-1.5 rounded-full active:opacity-70"
                style={{
                  backgroundColor: active
                    ? colors.accent.default
                    : `${colors.accent.default}15`,
                }}
              >
                <Text
                  className="text-xs font-semibold"
                  style={{
                    color: active ? "#fff" : colors.accent.hover,
                  }}
                >
                  {f.label}
                </Text>
              </Pressable>
            );
          })}
        </ScrollView>

        {/* Count header */}
        {connected && byPeer && (
          <View className="px-4 pb-3 flex-row items-center justify-between">
            <Text className="text-text-secondary text-sm font-semibold uppercase tracking-wider">
              Ideas
            </Text>
            <Text className="text-text-muted text-xs">
              {filter === "all"
                ? `${totalCount} total`
                : `${filteredCount} of ${totalCount}`}
            </Text>
          </View>
        )}

        {/* Content */}
        <View className="px-4">
          {!connected ? (
            <View className="bg-bg-card rounded-card p-6 items-center">
              <Text className="text-3xl mb-3">{tabIcons.ideas}</Text>
              <Text className="text-text-muted text-sm text-center">
                Connect to a peer to browse ideas
              </Text>
            </View>
          ) : (
            <PeerTree
              data={filteredByPeer}
              renderItem={(idea) => (
                <IdeaRow idea={idea} onDelete={handleDelete} />
              )}
              keyExtractor={(i) => i.id}
              emptyIcon={tabIcons.ideas}
              emptyMessage={
                filter === "all" ? "No ideas yet" : `No ${filter} ideas`
              }
              loading={isLoading}
            />
          )}
        </View>
      </ScrollView>

      {/* FAB — Create Idea (quick capture) */}
      <Pressable
        className="absolute right-5 bottom-6 w-14 h-14 rounded-full items-center justify-center active:opacity-70"
        style={{
          backgroundColor: colors.accent.default,
          shadowColor: colors.accent.default,
          shadowOffset: { width: 0, height: 4 },
          shadowOpacity: 0.3,
          shadowRadius: 8,
          elevation: 8,
        }}
        onPress={() => router.push("/ideas/create")}
      >
        <Text
          className="text-white text-2xl font-light"
          style={{ marginTop: -2 }}
        >
          +
        </Text>
      </Pressable>
    </View>
  );
}
