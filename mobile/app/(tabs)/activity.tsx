import { useCallback, useState } from "react";
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
import * as Haptics from "expo-haptics";
import { colors, tabIcons } from "@/lib/theme";
import { useAppStore } from "@/lib/store";
import {
  useActivity,
  useRunningTasks,
  useKillTask,
  queryKeys,
} from "@/lib/hooks";
import { RunningTasks } from "@/components/RunningTasks";
import { api } from "@/lib/api";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleTimeString("en", { hour: "2-digit", minute: "2-digit" });
  } catch {
    return "";
  }
}

function formatDateGroup(iso: string): string {
  try {
    const d = new Date(iso);
    const today = new Date();
    const yesterday = new Date(today);
    yesterday.setDate(yesterday.getDate() - 1);

    if (d.toDateString() === today.toDateString()) return "Today";
    if (d.toDateString() === yesterday.toDateString()) return "Yesterday";

    const month = d.toLocaleString("en", { month: "short" });
    return `${month} ${d.getDate()}`;
  } catch {
    return "";
  }
}

// Activity items from the API are generic objects — detect type from shape
interface ActivityItem {
  id?: string;
  type?: string;
  // Memory fields
  l0?: string;
  path?: string;
  source_node?: string;
  source_agent?: string;
  // Conversation fields
  title?: string;
  summary?: string;
  agent?: string;
  turn_count?: number;
  // Task fields
  status?: string;
  marker?: string;
  manifest_id?: string;
  run_count?: number;
  // Shared
  created_at?: string;
  updated_at?: string;
}

type ItemKind = "memory" | "conversation" | "task" | "unknown";

function detectKind(item: ActivityItem): ItemKind {
  if (item.type === "memory" || item.l0 !== undefined) return "memory";
  if (item.type === "conversation" || item.turn_count !== undefined)
    return "conversation";
  if (item.type === "task" || item.manifest_id !== undefined) return "task";
  return "unknown";
}

const kindIcons: Record<ItemKind, string> = {
  memory: tabIcons.memories,
  conversation: "\u2630", // ☰
  task: tabIcons.tasks,
  unknown: "\u2022", // •
};

const kindColors: Record<ItemKind, string> = {
  memory: "#8b5cf6",
  conversation: "#3b82f6",
  task: "#f59e0b",
  unknown: colors.text.muted,
};

const kindLabels: Record<ItemKind, string> = {
  memory: "Memory",
  conversation: "Conversation",
  task: "Task",
  unknown: "Event",
};

function getItemTitle(item: ActivityItem, kind: ItemKind): string {
  switch (kind) {
    case "memory":
      return item.l0 || item.path || "Memory";
    case "conversation":
      return item.title || item.summary || "Conversation";
    case "task":
      return item.title || item.marker || "Task";
    default:
      return item.title || item.l0 || "Activity";
  }
}

function getItemSubtitle(item: ActivityItem, kind: ItemKind): string {
  switch (kind) {
    case "memory":
      return item.source_agent
        ? `${item.source_agent} on ${item.source_node || "unknown"}`
        : item.source_node || "";
    case "conversation":
      return item.agent
        ? `${item.agent}${item.turn_count ? ` \u00B7 ${item.turn_count} turns` : ""}`
        : "";
    case "task":
      return item.status
        ? `${item.status}${item.run_count ? ` \u00B7 ${item.run_count} runs` : ""}`
        : "";
    default:
      return item.source_node || "";
  }
}

function getItemTimestamp(item: ActivityItem): string {
  return item.updated_at || item.created_at || "";
}

function getItemRoute(
  item: ActivityItem,
  kind: ItemKind,
): string | null {
  if (!item.id) return null;
  switch (kind) {
    case "memory":
      return `/memories/${item.id}`;
    case "task":
      return `/tasks/${item.id}`;
    default:
      return null;
  }
}

// ---------------------------------------------------------------------------
// Activity Row
// ---------------------------------------------------------------------------

function ActivityRow({ item }: { item: ActivityItem }) {
  const router = useRouter();
  const kind = detectKind(item);
  const route = getItemRoute(item, kind);
  const timestamp = getItemTimestamp(item);

  return (
    <Pressable
      className="flex-row py-3 px-4 active:opacity-70"
      style={{
        backgroundColor: colors.bg.card,
        borderBottomWidth: 1,
        borderBottomColor: colors.border.default,
      }}
      onPress={() => {
        if (route) router.push(route as never);
      }}
      disabled={!route}
    >
      {/* Icon */}
      <View
        className="w-8 h-8 rounded-full items-center justify-center mr-3"
        style={{ backgroundColor: `${kindColors[kind]}20` }}
      >
        <Text style={{ color: kindColors[kind], fontSize: 14 }}>
          {kindIcons[kind]}
        </Text>
      </View>

      {/* Content */}
      <View className="flex-1">
        <View className="flex-row items-center gap-2">
          <View
            className="px-1.5 py-0.5 rounded"
            style={{ backgroundColor: `${kindColors[kind]}20` }}
          >
            <Text
              className="text-xs font-medium"
              style={{ color: kindColors[kind] }}
            >
              {kindLabels[kind]}
            </Text>
          </View>
          {timestamp && (
            <Text className="text-text-muted text-xs">
              {formatTime(timestamp)}
            </Text>
          )}
        </View>
        <Text
          className="text-text-primary text-sm font-medium mt-1"
          numberOfLines={1}
        >
          {getItemTitle(item, kind)}
        </Text>
        {getItemSubtitle(item, kind) ? (
          <Text className="text-text-muted text-xs mt-0.5" numberOfLines={1}>
            {getItemSubtitle(item, kind)}
          </Text>
        ) : null}
      </View>

      {/* Chevron if navigable */}
      {route && (
        <View className="justify-center ml-2">
          <Text className="text-text-muted text-xs">{"\u203A"}</Text>
        </View>
      )}
    </Pressable>
  );
}

// ---------------------------------------------------------------------------
// Activity Screen
// ---------------------------------------------------------------------------

export default function ActivityScreen() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const connected = useAppStore((s) => s.connected);

  const { data: activity, isLoading } = useActivity();
  const { data: runningTasks } = useRunningTasks();
  const killMutation = useKillTask();

  const [refreshing, setRefreshing] = useState(false);
  const [killingId, setKillingId] = useState<string | null>(null);
  const [stoppingAll, setStoppingAll] = useState(false);

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    try {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.activity }),
        queryClient.invalidateQueries({ queryKey: queryKeys.runningTasks }),
      ]);
    } finally {
      setRefreshing(false);
    }
  }, [queryClient]);

  const handleKill = useCallback(
    (taskId: string) => {
      setKillingId(taskId);
      Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Heavy);
      killMutation.mutate(taskId, {
        onSettled: () => setKillingId(null),
      });
    },
    [killMutation],
  );

  const handleStopAll = useCallback(() => {
    if (!runningTasks?.length) return;

    Alert.alert(
      "Emergency Stop All",
      `Kill ${runningTasks.length} running task${runningTasks.length === 1 ? "" : "s"}?`,
      [
        { text: "Cancel", style: "cancel" },
        {
          text: "Kill All",
          style: "destructive",
          onPress: async () => {
            setStoppingAll(true);
            Haptics.notificationAsync(Haptics.NotificationFeedbackType.Warning);
            try {
              await Promise.all(runningTasks.map((t) => api.killTask(t.id)));
              queryClient.invalidateQueries({
                queryKey: queryKeys.runningTasks,
              });
              queryClient.invalidateQueries({ queryKey: queryKeys.tasks });
            } finally {
              setStoppingAll(false);
            }
          },
        },
      ],
    );
  }, [runningTasks, queryClient]);

  // Group activity items by date
  const activityItems = (activity ?? []) as ActivityItem[];
  const groupedByDate: Record<string, ActivityItem[]> = {};
  for (const item of activityItems) {
    const ts = getItemTimestamp(item);
    const key = ts ? formatDateGroup(ts) : "Unknown";
    if (!groupedByDate[key]) groupedByDate[key] = [];
    groupedByDate[key].push(item);
  }

  const runningCount = runningTasks?.length ?? 0;

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
        {/* Running Tasks Section */}
        {connected && (
          <View className="px-4 pt-4 mb-4">
            <View className="flex-row items-center justify-between mb-3">
              <Text className="text-text-secondary text-sm font-semibold uppercase tracking-wider">
                Running Tasks
              </Text>
              {runningCount > 0 && (
                <View
                  className="px-2 py-0.5 rounded-full"
                  style={{ backgroundColor: `${colors.status.success}22` }}
                >
                  <Text
                    style={{ color: colors.status.success }}
                    className="text-xs font-bold"
                  >
                    {runningCount}
                  </Text>
                </View>
              )}
            </View>
            <RunningTasks
              tasks={runningTasks ?? []}
              onKill={handleKill}
              killing={killingId}
            />

            {/* Emergency Stop All */}
            {runningCount > 0 && (
              <Pressable
                className="rounded-card p-3 items-center mt-3 active:opacity-70"
                style={{ backgroundColor: `${colors.status.error}33` }}
                onPress={handleStopAll}
                disabled={stoppingAll}
              >
                <Text
                  style={{ color: colors.status.error }}
                  className="text-sm font-bold"
                >
                  {stoppingAll
                    ? "Stopping All..."
                    : `Emergency Stop All (${runningCount})`}
                </Text>
              </Pressable>
            )}
          </View>
        )}

        {/* Timeline */}
        <View className="px-4">
          {!connected ? (
            <View className="bg-bg-card rounded-card p-6 items-center">
              <Text className="text-3xl mb-3">{tabIcons.activity}</Text>
              <Text className="text-text-muted text-sm text-center">
                Connect to a peer to see activity
              </Text>
            </View>
          ) : isLoading ? (
            <View className="bg-bg-card rounded-card p-6 items-center">
              <Text className="text-text-muted text-sm">Loading...</Text>
            </View>
          ) : activityItems.length === 0 ? (
            <View className="bg-bg-card rounded-card p-6 items-center">
              <Text className="text-3xl mb-3">{tabIcons.activity}</Text>
              <Text className="text-text-muted text-sm">
                No activity yet
              </Text>
            </View>
          ) : (
            Object.entries(groupedByDate).map(([dateLabel, items]) => (
              <View key={dateLabel} className="mb-4">
                <Text className="text-text-secondary text-sm font-semibold uppercase tracking-wider mb-2">
                  {dateLabel}
                </Text>
                <View className="bg-bg-card rounded-card overflow-hidden">
                  {items.map((item, index) => (
                    <ActivityRow
                      key={item.id ?? `activity-${index}`}
                      item={item}
                    />
                  ))}
                </View>
              </View>
            ))
          )}
        </View>
      </ScrollView>
    </View>
  );
}
