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
  useTasksByPeer,
  useManifests,
  useDeleteTask,
  queryKeys,
} from "@/lib/hooks";
import { StatusBadge } from "@/components/StatusBadge";
import type { Task, TaskStatus, Manifest } from "@/lib/types";

// ---------------------------------------------------------------------------
// Status filter
// ---------------------------------------------------------------------------

type FilterOption = "all" | TaskStatus;

const FILTERS: { key: FilterOption; label: string }[] = [
  { key: "all", label: "All" },
  { key: "running", label: "Running" },
  { key: "scheduled", label: "Scheduled" },
  { key: "pending", label: "Pending" },
  { key: "completed", label: "Done" },
  { key: "failed", label: "Failed" },
  { key: "cancelled", label: "Cancelled" },
];

const taskStatusMap: Record<
  TaskStatus,
  "running" | "success" | "warning" | "error" | "idle"
> = {
  pending: "idle",
  scheduled: "warning",
  running: "running",
  completed: "success",
  failed: "error",
  cancelled: "idle",
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
// Task Row (with swipe-to-delete)
// ---------------------------------------------------------------------------

function TaskRow({
  task,
  manifestTitle,
  onDelete,
}: {
  task: Task;
  manifestTitle: string;
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
        onDelete(task.id);
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
        onPress={() => router.push(`/tasks/${task.id}`)}
      >
        <View className="flex-row items-center justify-between">
          <Text
            className="text-text-primary text-sm font-medium flex-1 mr-3"
            numberOfLines={1}
          >
            {task.title}
          </Text>
          <Text className="text-text-muted text-xs">
            {formatDate(task.created_at)}
          </Text>
        </View>

        <View className="flex-row items-center mt-1.5 gap-2">
          <Text
            className="text-text-muted text-xs"
            style={{ fontFamily: "Courier" }}
          >
            {task.marker}
          </Text>
          <StatusBadge
            status={taskStatusMap[task.status]}
            label={task.status}
          />
          {task.schedule !== "once" && (
            <Text className="text-text-muted text-xs">
              {task.schedule}
            </Text>
          )}
        </View>

        {/* Manifest + meta line */}
        <View className="flex-row items-center mt-1 gap-2">
          {manifestTitle ? (
            <Text
              className="text-xs flex-1"
              style={{ color: colors.accent.hover }}
              numberOfLines={1}
            >
              {manifestTitle}
            </Text>
          ) : null}
          {task.max_turns > 0 && (
            <Text className="text-text-muted text-xs">
              {task.max_turns} turns
            </Text>
          )}
          {task.run_count > 0 && (
            <Text className="text-text-muted text-xs">
              {task.run_count} runs
            </Text>
          )}
        </View>
      </Pressable>
    </Swipeable>
  );
}

// ---------------------------------------------------------------------------
// Manifest group header for Peer > Manifest > Task tree
// ---------------------------------------------------------------------------

function ManifestGroup({
  manifestTitle,
  tasks,
  onDeleteTask,
  manifestLookup,
}: {
  manifestTitle: string;
  tasks: Task[];
  onDeleteTask: (id: string) => void;
  manifestLookup: Map<string, Manifest>;
}) {
  const [expanded, setExpanded] = useState(true);

  return (
    <View className="mb-2">
      <Pressable
        onPress={() => setExpanded(!expanded)}
        className="flex-row items-center py-2 px-3 active:opacity-70"
        style={{
          backgroundColor: `${colors.accent.default}08`,
          borderTopLeftRadius: 8,
          borderTopRightRadius: 8,
          borderBottomLeftRadius: expanded ? 0 : 8,
          borderBottomRightRadius: expanded ? 0 : 8,
        }}
      >
        <Text className="text-text-muted text-xs mr-2">
          {expanded ? "\u25BC" : "\u25B6"}
        </Text>
        <Text
          className="text-xs font-semibold flex-1"
          style={{ color: colors.accent.hover }}
          numberOfLines={1}
        >
          {manifestTitle || "No Manifest"}
        </Text>
        <View
          className="px-1.5 py-0.5 rounded-full"
          style={{ backgroundColor: `${colors.accent.default}22` }}
        >
          <Text
            style={{ color: colors.accent.default }}
            className="text-xs font-bold"
          >
            {tasks.length}
          </Text>
        </View>
      </Pressable>
      {expanded &&
        tasks.map((task, index) => (
          <View
            key={task.id}
            style={
              index < tasks.length - 1
                ? {
                    borderBottomWidth: 1,
                    borderBottomColor: colors.border.default,
                  }
                : {
                    borderBottomLeftRadius: 8,
                    borderBottomRightRadius: 8,
                    overflow: "hidden",
                  }
            }
          >
            <TaskRow
              task={task}
              manifestTitle=""
              onDelete={onDeleteTask}
            />
          </View>
        ))}
    </View>
  );
}

// ---------------------------------------------------------------------------
// Peer section — groups tasks by manifest under each peer
// ---------------------------------------------------------------------------

function PeerTaskSection({
  peer,
  tasks,
  manifestLookup,
  onDeleteTask,
}: {
  peer: string;
  tasks: Task[];
  manifestLookup: Map<string, Manifest>;
  onDeleteTask: (id: string) => void;
}) {
  const [expanded, setExpanded] = useState(true);

  // Group tasks by manifest_id
  const byManifest = useMemo(() => {
    const groups = new Map<string, Task[]>();
    for (const task of tasks) {
      const key = task.manifest_id || "__none__";
      const list = groups.get(key) ?? [];
      list.push(task);
      groups.set(key, list);
    }
    return groups;
  }, [tasks]);

  return (
    <View className="mb-3">
      <Pressable
        onPress={() => setExpanded(!expanded)}
        className="flex-row items-center py-3 px-4 active:opacity-70"
        style={{
          backgroundColor: colors.bg.card,
          borderTopLeftRadius: 12,
          borderTopRightRadius: 12,
          borderBottomLeftRadius: expanded ? 0 : 12,
          borderBottomRightRadius: expanded ? 0 : 12,
        }}
      >
        <Text className="text-text-muted text-xs mr-2">
          {expanded ? "\u25BC" : "\u25B6"}
        </Text>
        <Text
          className="text-text-secondary text-sm font-medium flex-1"
          numberOfLines={1}
        >
          {peer}
        </Text>
        <View
          className="px-2 py-0.5 rounded-full"
          style={{ backgroundColor: `${colors.accent.default}22` }}
        >
          <Text
            style={{ color: colors.accent.default }}
            className="text-xs font-bold"
          >
            {tasks.length}
          </Text>
        </View>
      </Pressable>
      {expanded && (
        <View
          style={{
            backgroundColor: colors.bg.card,
            borderTopWidth: 1,
            borderTopColor: colors.border.default,
            borderBottomLeftRadius: 12,
            borderBottomRightRadius: 12,
            paddingHorizontal: 4,
            paddingVertical: 4,
          }}
        >
          {Array.from(byManifest.entries()).map(([manifestId, groupTasks]) => {
            const manifest = manifestLookup.get(manifestId);
            return (
              <ManifestGroup
                key={manifestId}
                manifestTitle={manifest?.title ?? manifestId}
                tasks={groupTasks}
                onDeleteTask={onDeleteTask}
                manifestLookup={manifestLookup}
              />
            );
          })}
        </View>
      )}
    </View>
  );
}

// ---------------------------------------------------------------------------
// Tasks List Screen
// ---------------------------------------------------------------------------

export default function TasksListScreen() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const connected = useAppStore((s) => s.connected);

  const { data: byPeer, isLoading, refetch } = useTasksByPeer();
  const { data: manifests } = useManifests();
  const deleteMutation = useDeleteTask();

  const [filter, setFilter] = useState<FilterOption>("all");
  const [refreshing, setRefreshing] = useState(false);

  // Build manifest lookup for titles
  const manifestLookup = useMemo(() => {
    const map = new Map<string, Manifest>();
    for (const m of manifests ?? []) {
      map.set(m.id, m);
    }
    return map;
  }, [manifests]);

  // Filter tasks by status (client-side)
  const filteredByPeer = useMemo(() => {
    if (!byPeer || filter === "all") return byPeer;
    const result: Record<string, Task[]> = {};
    for (const [peer, tasks] of Object.entries(byPeer)) {
      const filtered = tasks.filter((t) => t.status === filter);
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
        queryKey: queryKeys.tasksByPeer,
      });
    } finally {
      setRefreshing(false);
    }
  }, [queryClient]);

  const handleDelete = useCallback(
    (id: string) => {
      const task = Object.values(byPeer ?? {})
        .flat()
        .find((t) => t.id === id);
      const title = task?.title ?? "this task";

      Alert.alert(
        "Delete Task",
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

  // Counts
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
              Tasks
            </Text>
            <Text className="text-text-muted text-xs">
              {filter === "all"
                ? `${totalCount} total`
                : `${filteredCount} of ${totalCount}`}
            </Text>
          </View>
        )}

        {/* Content — Peer > Manifest > Task tree */}
        <View className="px-4">
          {!connected ? (
            <View className="bg-bg-card rounded-card p-6 items-center">
              <Text className="text-3xl mb-3">{tabIcons.tasks}</Text>
              <Text className="text-text-muted text-sm text-center">
                Connect to a peer to browse tasks
              </Text>
            </View>
          ) : !filteredByPeer ||
            Object.keys(filteredByPeer).length === 0 ? (
            <View className="bg-bg-card rounded-card p-6 items-center">
              <Text className="text-3xl mb-3">{tabIcons.tasks}</Text>
              <Text className="text-text-muted text-sm">
                {filter === "all"
                  ? "No tasks yet"
                  : `No ${filter} tasks`}
              </Text>
            </View>
          ) : (
            Object.entries(filteredByPeer).map(([peer, tasks]) => (
              <PeerTaskSection
                key={peer}
                peer={peer}
                tasks={tasks}
                manifestLookup={manifestLookup}
                onDeleteTask={handleDelete}
              />
            ))
          )}
        </View>
      </ScrollView>

      {/* FAB — Create Task */}
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
        onPress={() => router.push("/tasks/create")}
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
