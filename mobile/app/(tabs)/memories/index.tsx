import { useCallback, useMemo, useState } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  RefreshControl,
  TextInput,
  Alert,
  ActivityIndicator,
} from "react-native";
import { useRouter } from "expo-router";
import { useQueryClient } from "@tanstack/react-query";
import Swipeable from "react-native-gesture-handler/Swipeable";
import * as Haptics from "expo-haptics";
import { colors, tabIcons } from "@/lib/theme";
import { useAppStore } from "@/lib/store";
import {
  useMemoriesBySession,
  useSearchMemories,
  useDeleteMemory,
  queryKeys,
} from "@/lib/hooks";
import { PeerTree } from "@/components/PeerTree";
import type { Memory, MemoryType } from "@/lib/types";
import { useRef } from "react";

// ---------------------------------------------------------------------------
// Type filter
// ---------------------------------------------------------------------------

type FilterOption = "all" | MemoryType;

const FILTERS: { key: FilterOption; label: string }[] = [
  { key: "all", label: "All" },
  { key: "insight", label: "Insight" },
  { key: "decision", label: "Decision" },
  { key: "pattern", label: "Pattern" },
  { key: "bug", label: "Bug" },
  { key: "context", label: "Context" },
  { key: "reference", label: "Reference" },
  { key: "visceral", label: "Visceral" },
];

const typeColors: Record<MemoryType, string> = {
  insight: "#8b5cf6",
  decision: "#3b82f6",
  pattern: "#06b6d4",
  bug: "#e63757",
  context: "#f59e0b",
  reference: "#10b981",
  visceral: "#e63757",
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
// Memory Row (with swipe-to-delete)
// ---------------------------------------------------------------------------

function MemoryRow({
  memory,
  onDelete,
}: {
  memory: Memory;
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
        onDelete(memory.id);
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
        onPress={() => router.push(`/memories/${memory.id}`)}
      >
        {/* Title line: L0 summary + date */}
        <View className="flex-row items-center justify-between">
          <Text
            className="text-text-primary text-sm font-medium flex-1 mr-3"
            numberOfLines={1}
          >
            {memory.l0 || memory.path}
          </Text>
          <Text className="text-text-muted text-xs">
            {formatDate(memory.created_at)}
          </Text>
        </View>

        {/* Meta line: type badge + scope + access count */}
        <View className="flex-row items-center mt-1.5 gap-2">
          <View
            className="px-2 py-0.5 rounded-full"
            style={{
              backgroundColor: `${typeColors[memory.type] ?? colors.text.muted}20`,
            }}
          >
            <Text
              className="text-xs font-medium"
              style={{ color: typeColors[memory.type] ?? colors.text.muted }}
            >
              {memory.type}
            </Text>
          </View>
          <Text className="text-text-muted text-xs">{memory.scope}</Text>
          {memory.access_count > 0 && (
            <Text className="text-text-muted text-xs">
              {memory.access_count}x
            </Text>
          )}
        </View>

        {/* Tags */}
        {memory.tags.length > 0 && (
          <View className="flex-row flex-wrap gap-1.5 mt-2">
            {memory.tags.slice(0, 3).map((tag) => (
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
            {memory.tags.length > 3 && (
              <Text className="text-text-muted text-xs self-center">
                +{memory.tags.length - 3}
              </Text>
            )}
          </View>
        )}
      </Pressable>
    </Swipeable>
  );
}

// ---------------------------------------------------------------------------
// Search Result Row
// ---------------------------------------------------------------------------

function SearchResultRow({
  memory,
  score,
}: {
  memory: Memory;
  score: number;
}) {
  const router = useRouter();
  return (
    <Pressable
      className="py-3 px-4 active:opacity-70"
      style={{
        backgroundColor: colors.bg.card,
        borderBottomWidth: 1,
        borderBottomColor: colors.border.default,
      }}
      onPress={() => router.push(`/memories/${memory.id}`)}
    >
      <View className="flex-row items-center justify-between">
        <Text
          className="text-text-primary text-sm font-medium flex-1 mr-3"
          numberOfLines={1}
        >
          {memory.l0 || memory.path}
        </Text>
        <View
          className="px-2 py-0.5 rounded-full"
          style={{ backgroundColor: `${colors.status.success}20` }}
        >
          <Text
            className="text-xs font-medium"
            style={{ color: colors.status.success }}
          >
            {Math.round(score * 100)}%
          </Text>
        </View>
      </View>
      <View className="flex-row items-center mt-1 gap-2">
        <View
          className="px-2 py-0.5 rounded-full"
          style={{
            backgroundColor: `${typeColors[memory.type] ?? colors.text.muted}20`,
          }}
        >
          <Text
            className="text-xs font-medium"
            style={{ color: typeColors[memory.type] ?? colors.text.muted }}
          >
            {memory.type}
          </Text>
        </View>
        <Text className="text-text-muted text-xs" numberOfLines={1}>
          {memory.l1 ? memory.l1.slice(0, 80) : memory.l0}
        </Text>
      </View>
    </Pressable>
  );
}

// ---------------------------------------------------------------------------
// Memories List Screen
// ---------------------------------------------------------------------------

export default function MemoriesListScreen() {
  const queryClient = useQueryClient();
  const connected = useAppStore((s) => s.connected);

  const { data: bySession, isLoading, refetch } = useMemoriesBySession();
  const deleteMutation = useDeleteMemory();
  const searchMutation = useSearchMemories();

  const [filter, setFilter] = useState<FilterOption>("all");
  const [refreshing, setRefreshing] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [isSearching, setIsSearching] = useState(false);

  // Sort memories within each session newest first, then filter by type
  const filteredBySession = useMemo(() => {
    if (!bySession) return bySession;

    const result: Record<string, Memory[]> = {};
    for (const [session, memories] of Object.entries(bySession)) {
      let sorted = [...memories].sort(
        (a, b) =>
          new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
      );
      if (filter !== "all") {
        sorted = sorted.filter((m) => m.type === filter);
      }
      if (sorted.length > 0) {
        result[session] = sorted;
      }
    }
    return result;
  }, [bySession, filter]);

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    try {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.memoriesBySession,
      });
    } finally {
      setRefreshing(false);
    }
  }, [queryClient]);

  const handleDelete = useCallback(
    (id: string) => {
      const memory = Object.values(bySession ?? {})
        .flat()
        .find((m) => m.id === id);
      const title = memory?.l0 ?? "this memory";

      Alert.alert(
        "Delete Memory",
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
    [bySession, deleteMutation],
  );

  const handleSearch = useCallback(() => {
    if (!searchQuery.trim()) {
      setIsSearching(false);
      return;
    }
    setIsSearching(true);
    searchMutation.mutate({ query: searchQuery.trim(), limit: 20 });
  }, [searchQuery, searchMutation]);

  const clearSearch = useCallback(() => {
    setSearchQuery("");
    setIsSearching(false);
  }, []);

  // Totals
  const totalCount = Object.values(bySession ?? {}).reduce(
    (sum, arr) => sum + arr.length,
    0,
  );
  const filteredCount = Object.values(filteredBySession ?? {}).reduce(
    (sum, arr) => sum + arr.length,
    0,
  );

  return (
    <View className="flex-1 bg-bg-primary">
      <ScrollView
        className="flex-1"
        contentContainerStyle={{ paddingBottom: 40 }}
        keyboardShouldPersistTaps="handled"
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            onRefresh={onRefresh}
            tintColor={colors.text.muted}
          />
        }
      >
        {/* Search Bar */}
        <View className="px-4 pt-4 pb-2">
          <View
            className="flex-row items-center rounded-lg px-3"
            style={{
              backgroundColor: colors.bg.input,
              borderColor: colors.border.default,
              borderWidth: 1,
            }}
          >
            <Text className="text-text-muted text-sm mr-2">{"\u2315"}</Text>
            <TextInput
              className="flex-1 py-2.5 text-text-primary text-sm"
              placeholder="Search memories..."
              placeholderTextColor={colors.text.muted}
              value={searchQuery}
              onChangeText={setSearchQuery}
              onSubmitEditing={handleSearch}
              returnKeyType="search"
            />
            {searchQuery.length > 0 && (
              <Pressable onPress={clearSearch} className="p-1 active:opacity-70">
                <Text className="text-text-muted text-sm">{"\u2715"}</Text>
              </Pressable>
            )}
          </View>
        </View>

        {/* Search Results */}
        {isSearching && (
          <View className="px-4 pb-3">
            {searchMutation.isPending && (
              <View className="py-6 items-center">
                <ActivityIndicator color={colors.accent.default} />
              </View>
            )}
            {searchMutation.data && (
              <>
                <View className="flex-row items-center justify-between mb-2">
                  <Text className="text-text-secondary text-sm font-semibold uppercase tracking-wider">
                    Search Results
                  </Text>
                  <Text className="text-text-muted text-xs">
                    {searchMutation.data.length} found
                  </Text>
                </View>
                <View className="bg-bg-card rounded-card overflow-hidden">
                  {searchMutation.data.length === 0 ? (
                    <View className="p-6 items-center">
                      <Text className="text-text-muted text-sm">
                        No memories match your search
                      </Text>
                    </View>
                  ) : (
                    searchMutation.data.map((result) => (
                      <SearchResultRow
                        key={result.memory.id}
                        memory={result.memory}
                        score={result.score}
                      />
                    ))
                  )}
                </View>
                <Pressable
                  onPress={clearSearch}
                  className="mt-2 py-2 items-center active:opacity-70"
                >
                  <Text
                    className="text-xs font-medium"
                    style={{ color: colors.accent.default }}
                  >
                    Clear search
                  </Text>
                </Pressable>
              </>
            )}
          </View>
        )}

        {/* Type Filter Chips (hidden during search) */}
        {!isSearching && (
          <>
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
                    : typeColors[f.key as MemoryType] ?? colors.accent.default;
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
                      style={{
                        color: active ? "#fff" : chipColor,
                      }}
                    >
                      {f.label}
                    </Text>
                  </Pressable>
                );
              })}
            </ScrollView>

            {/* Count header */}
            {connected && bySession && (
              <View className="px-4 pb-3 flex-row items-center justify-between">
                <Text className="text-text-secondary text-sm font-semibold uppercase tracking-wider">
                  By Session
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
                  <Text className="text-3xl mb-3">{tabIcons.memories}</Text>
                  <Text className="text-text-muted text-sm text-center">
                    Connect to a peer to browse memories
                  </Text>
                </View>
              ) : (
                <PeerTree
                  data={filteredBySession}
                  renderItem={(memory) => (
                    <MemoryRow memory={memory} onDelete={handleDelete} />
                  )}
                  keyExtractor={(m) => m.id}
                  emptyIcon={tabIcons.memories}
                  emptyMessage={
                    filter === "all"
                      ? "No memories yet"
                      : `No ${filter} memories`
                  }
                  loading={isLoading}
                />
              )}
            </View>
          </>
        )}
      </ScrollView>
    </View>
  );
}
