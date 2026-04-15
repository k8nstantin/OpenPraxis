import { useCallback, useMemo, useState } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  RefreshControl,
  Alert,
  ActivityIndicator,
} from "react-native";
import { useQueryClient } from "@tanstack/react-query";
import * as Haptics from "expo-haptics";
import { colors, tabIcons } from "@/lib/theme";
import { useAppStore } from "@/lib/store";
import { useRecall, useRestore, queryKeys } from "@/lib/hooks";
import type { RecallItem } from "@/lib/types";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type FilterOption = "all" | RecallItem["type"];

const FILTERS: { key: FilterOption; label: string }[] = [
  { key: "all", label: "All" },
  { key: "memory", label: "Memories" },
  { key: "manifest", label: "Manifests" },
  { key: "idea", label: "Ideas" },
  { key: "task", label: "Tasks" },
];

const typeIcons: Record<RecallItem["type"], string> = {
  memory: tabIcons.memories,
  manifest: tabIcons.manifests,
  idea: tabIcons.ideas,
  task: tabIcons.tasks,
};

const typeColors: Record<RecallItem["type"], string> = {
  memory: "#8b5cf6",
  manifest: "#3b82f6",
  idea: "#f59e0b",
  task: "#10b981",
};

// ---------------------------------------------------------------------------
// Recall Row
// ---------------------------------------------------------------------------

function RecallRow({
  item,
  onRestore,
  restoring,
}: {
  item: RecallItem;
  onRestore: (type: string, id: string) => void;
  restoring: string | null;
}) {
  const isRestoring = restoring === item.id;
  const color = typeColors[item.type] ?? colors.text.muted;

  return (
    <View
      className="flex-row items-center py-3 px-4"
      style={{
        backgroundColor: colors.bg.card,
        borderBottomWidth: 1,
        borderBottomColor: colors.border.default,
      }}
    >
      {/* Type icon */}
      <View
        className="w-8 h-8 rounded-full items-center justify-center mr-3"
        style={{ backgroundColor: `${color}20` }}
      >
        <Text style={{ color, fontSize: 14 }}>{typeIcons[item.type]}</Text>
      </View>

      {/* Content */}
      <View className="flex-1">
        <Text
          className="text-text-primary text-sm font-medium"
          numberOfLines={1}
        >
          {item.title}
        </Text>
        <View className="flex-row items-center mt-1 gap-2">
          <View
            className="px-1.5 py-0.5 rounded"
            style={{ backgroundColor: `${color}20` }}
          >
            <Text className="text-xs font-medium" style={{ color }}>
              {item.type}
            </Text>
          </View>
          {item.marker && (
            <Text
              className="text-text-muted text-xs"
              style={{ fontFamily: "Courier" }}
            >
              {item.marker}
            </Text>
          )}
        </View>
      </View>

      {/* Restore button */}
      <Pressable
        className="px-3 py-1.5 rounded-md active:opacity-70"
        style={{ backgroundColor: `${colors.status.success}22` }}
        onPress={() => onRestore(item.type, item.id)}
        disabled={isRestoring}
      >
        {isRestoring ? (
          <ActivityIndicator size="small" color={colors.status.success} />
        ) : (
          <Text
            className="text-xs font-semibold"
            style={{ color: colors.status.success }}
          >
            Restore
          </Text>
        )}
      </Pressable>
    </View>
  );
}

// ---------------------------------------------------------------------------
// Recall Screen
// ---------------------------------------------------------------------------

export default function RecallScreen() {
  const queryClient = useQueryClient();
  const connected = useAppStore((s) => s.connected);

  const { data: items, isLoading } = useRecall();
  const restoreMutation = useRestore();

  const [refreshing, setRefreshing] = useState(false);
  const [filter, setFilter] = useState<FilterOption>("all");
  const [restoringId, setRestoringId] = useState<string | null>(null);

  // Filter items by type
  const filteredItems = useMemo(() => {
    if (!items || filter === "all") return items;
    return items.filter((i) => i.type === filter);
  }, [items, filter]);

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    try {
      await queryClient.invalidateQueries({ queryKey: queryKeys.recall });
    } finally {
      setRefreshing(false);
    }
  }, [queryClient]);

  const handleRestore = useCallback(
    (type: string, id: string) => {
      const item = items?.find((i) => i.id === id);
      const title = item?.title ?? "this item";

      Alert.alert("Restore Item", `Restore "${title}"?`, [
        { text: "Cancel", style: "cancel" },
        {
          text: "Restore",
          onPress: () => {
            setRestoringId(id);
            Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
            restoreMutation.mutate(
              { type, id },
              { onSettled: () => setRestoringId(null) },
            );
          },
        },
      ]);
    },
    [items, restoreMutation],
  );

  const totalCount = items?.length ?? 0;
  const filteredCount = filteredItems?.length ?? 0;

  return (
    <View className="flex-1 bg-bg-primary">
      <ScrollView
        className="flex-1"
        contentContainerStyle={{ paddingBottom: 40 }}
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            onRefresh={onRefresh}
            tintColor={colors.text.muted}
          />
        }
      >
        {/* Header */}
        <View className="px-4 pt-4 pb-2">
          <Text className="text-text-muted text-xs">
            Soft-deleted items. Tap Restore to bring them back.
          </Text>
        </View>

        {/* Type Filter Chips */}
        <ScrollView
          horizontal
          showsHorizontalScrollIndicator={false}
          className="px-4 pb-3"
          contentContainerStyle={{ gap: 8 }}
        >
          {FILTERS.map((f) => {
            const active = filter === f.key;
            const chipColor =
              f.key === "all"
                ? colors.accent.default
                : typeColors[f.key as RecallItem["type"]] ??
                  colors.accent.default;
            return (
              <Pressable
                key={f.key}
                onPress={() => setFilter(f.key)}
                className="px-3.5 py-1.5 rounded-full active:opacity-70"
                style={{
                  backgroundColor: active ? chipColor : `${chipColor}15`,
                }}
              >
                <Text
                  className="text-xs font-semibold"
                  style={{ color: active ? "#fff" : chipColor }}
                >
                  {f.label}
                </Text>
              </Pressable>
            );
          })}
        </ScrollView>

        {/* Count header */}
        {connected && items && (
          <View className="px-4 pb-3 flex-row items-center justify-between">
            <Text className="text-text-secondary text-sm font-semibold uppercase tracking-wider">
              Deleted Items
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
              <Text className="text-3xl mb-3">{tabIcons.recall}</Text>
              <Text className="text-text-muted text-sm text-center">
                Connect to a peer to view deleted items
              </Text>
            </View>
          ) : isLoading ? (
            <View className="py-12 items-center">
              <ActivityIndicator color={colors.accent.default} />
            </View>
          ) : !filteredItems || filteredItems.length === 0 ? (
            <View className="bg-bg-card rounded-card p-6 items-center">
              <Text className="text-3xl mb-3">{tabIcons.recall}</Text>
              <Text className="text-text-muted text-sm">
                {filter === "all"
                  ? "No deleted items"
                  : `No deleted ${filter}s`}
              </Text>
            </View>
          ) : (
            <View className="bg-bg-card rounded-card overflow-hidden">
              {filteredItems.map((item) => (
                <RecallRow
                  key={item.id}
                  item={item}
                  onRestore={handleRestore}
                  restoring={restoringId}
                />
              ))}
            </View>
          )}
        </View>
      </ScrollView>
    </View>
  );
}
