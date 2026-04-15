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
  useManifestsByPeer,
  useDeleteManifest,
  queryKeys,
} from "@/lib/hooks";
import { PeerTree } from "@/components/PeerTree";
import { StatusBadge } from "@/components/StatusBadge";
import type { Manifest, ManifestStatus } from "@/lib/types";

// ---------------------------------------------------------------------------
// Status filter
// ---------------------------------------------------------------------------

type FilterOption = "all" | ManifestStatus;

const FILTERS: { key: FilterOption; label: string }[] = [
  { key: "all", label: "All" },
  { key: "draft", label: "Draft" },
  { key: "active", label: "Active" },
  { key: "completed", label: "Done" },
  { key: "archived", label: "Archived" },
];

const manifestStatusMap: Record<ManifestStatus, "running" | "success" | "warning" | "error" | "idle"> = {
  draft: "idle",
  active: "running",
  completed: "success",
  archived: "warning",
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
// Manifest Row (with swipe-to-delete)
// ---------------------------------------------------------------------------

function ManifestRow({
  manifest,
  onDelete,
}: {
  manifest: Manifest;
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
        onDelete(manifest.id);
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
        onPress={() => router.push(`/manifests/${manifest.id}`)}
      >
        <View className="flex-row items-center justify-between">
          <Text
            className="text-text-primary text-sm font-medium flex-1 mr-3"
            numberOfLines={1}
          >
            {manifest.title}
          </Text>
          <Text className="text-text-muted text-xs">
            {formatDate(manifest.created_at)}
          </Text>
        </View>

        <View className="flex-row items-center mt-1.5 gap-2">
          <Text className="text-text-muted text-xs" style={{ fontFamily: "Courier" }}>
            {manifest.marker}
          </Text>
          <StatusBadge
            status={manifestStatusMap[manifest.status]}
            label={manifest.status}
          />
          {manifest.version > 1 && (
            <Text className="text-text-muted text-xs">v{manifest.version}</Text>
          )}
        </View>

        {manifest.tags.length > 0 && (
          <View className="flex-row flex-wrap gap-1.5 mt-2">
            {manifest.tags.slice(0, 4).map((tag) => (
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
            {manifest.tags.length > 4 && (
              <Text className="text-text-muted text-xs self-center">
                +{manifest.tags.length - 4}
              </Text>
            )}
          </View>
        )}
      </Pressable>
    </Swipeable>
  );
}

// ---------------------------------------------------------------------------
// Manifests List Screen
// ---------------------------------------------------------------------------

export default function ManifestsListScreen() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const connected = useAppStore((s) => s.connected);

  const { data: byPeer, isLoading, refetch } = useManifestsByPeer();
  const deleteMutation = useDeleteManifest();

  const [filter, setFilter] = useState<FilterOption>("all");
  const [refreshing, setRefreshing] = useState(false);

  // Filter manifests by status (client-side)
  const filteredByPeer = useMemo(() => {
    if (!byPeer || filter === "all") return byPeer;
    const result: Record<string, Manifest[]> = {};
    for (const [peer, manifests] of Object.entries(byPeer)) {
      const filtered = manifests.filter((m) => m.status === filter);
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
        queryKey: queryKeys.manifestsByPeer,
      });
    } finally {
      setRefreshing(false);
    }
  }, [queryClient]);

  const handleDelete = useCallback(
    (id: string) => {
      const manifest = Object.values(byPeer ?? {})
        .flat()
        .find((m) => m.id === id);
      const title = manifest?.title ?? "this manifest";

      Alert.alert("Delete Manifest", `Delete "${title}"? It can be restored from Recall.`, [
        { text: "Cancel", style: "cancel" },
        {
          text: "Delete",
          style: "destructive",
          onPress: () => {
            Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
            deleteMutation.mutate(id);
          },
        },
      ]);
    },
    [byPeer, deleteMutation],
  );

  // Total count for header
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
              Manifests
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
              <Text className="text-3xl mb-3">{tabIcons.manifests}</Text>
              <Text className="text-text-muted text-sm text-center">
                Connect to a peer to browse manifests
              </Text>
            </View>
          ) : (
            <PeerTree
              data={filteredByPeer}
              renderItem={(manifest) => (
                <ManifestRow manifest={manifest} onDelete={handleDelete} />
              )}
              keyExtractor={(m) => m.id}
              emptyIcon={tabIcons.manifests}
              emptyMessage={
                filter === "all"
                  ? "No manifests yet"
                  : `No ${filter} manifests`
              }
              loading={isLoading}
            />
          )}
        </View>
      </ScrollView>

      {/* FAB — Create Manifest */}
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
        onPress={() => router.push("/manifests/create")}
      >
        <Text className="text-white text-2xl font-light" style={{ marginTop: -2 }}>
          +
        </Text>
      </Pressable>
    </View>
  );
}
