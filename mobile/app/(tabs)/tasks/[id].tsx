import { useState } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  Alert,
  ActivityIndicator,
} from "react-native";
import { useLocalSearchParams, useRouter } from "expo-router";
import * as Haptics from "expo-haptics";
import { colors } from "@/lib/theme";
import {
  useTask,
  useTaskRuns,
  useTaskAmnesia,
  useTaskDelusions,
  useStartTask,
  useCancelTask,
  useKillTask,
  useDeleteTask,
} from "@/lib/hooks";
import { StatusBadge } from "@/components/StatusBadge";
import type { TaskStatus, TaskRun, Amnesia, Delusion } from "@/lib/types";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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

function formatDateTime(iso: string): string {
  if (!iso) return "\u2014";
  try {
    const d = new Date(iso);
    return d.toLocaleString("en", {
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
}

// ---------------------------------------------------------------------------
// Metadata row
// ---------------------------------------------------------------------------

function MetaRow({ label, value }: { label: string; value: string }) {
  if (!value || value === "\u2014") return null;
  return (
    <View className="flex-row py-1.5">
      <Text className="text-text-muted text-xs w-28">{label}</Text>
      <Text className="text-text-secondary text-xs flex-1" selectable>
        {value}
      </Text>
    </View>
  );
}

// ---------------------------------------------------------------------------
// Collapsible section
// ---------------------------------------------------------------------------

function Section({
  title,
  count,
  children,
  defaultExpanded = true,
}: {
  title: string;
  count?: number;
  children: React.ReactNode;
  defaultExpanded?: boolean;
}) {
  const [expanded, setExpanded] = useState(defaultExpanded);

  return (
    <View className="mb-4">
      <Pressable
        onPress={() => setExpanded(!expanded)}
        className="flex-row items-center py-2 active:opacity-70"
      >
        <Text className="text-text-muted text-xs mr-1.5">
          {expanded ? "\u25BC" : "\u25B6"}
        </Text>
        <Text className="text-text-secondary text-sm font-semibold uppercase tracking-wider flex-1">
          {title}
        </Text>
        {count != null && (
          <View
            className="px-2 py-0.5 rounded-full"
            style={{ backgroundColor: `${colors.accent.default}22` }}
          >
            <Text
              style={{ color: colors.accent.default }}
              className="text-xs font-bold"
            >
              {count}
            </Text>
          </View>
        )}
      </Pressable>
      {expanded && children}
    </View>
  );
}

// ---------------------------------------------------------------------------
// Run history row
// ---------------------------------------------------------------------------

function RunRow({ run }: { run: TaskRun }) {
  const statusColor =
    run.status === "completed"
      ? colors.status.success
      : run.status === "failed"
        ? colors.status.error
        : colors.text.muted;

  return (
    <View
      className="py-2.5 px-3"
      style={{
        borderBottomWidth: 1,
        borderBottomColor: colors.border.default,
      }}
    >
      <View className="flex-row items-center justify-between">
        <Text className="text-text-primary text-sm font-medium">
          Run #{run.run_number}
        </Text>
        <Text className="text-xs" style={{ color: statusColor }}>
          {run.status}
        </Text>
      </View>
      <View className="flex-row items-center gap-3 mt-1">
        <Text className="text-text-muted text-xs">
          {run.lines} lines
        </Text>
        <Text className="text-text-muted text-xs">
          {run.actions} actions
        </Text>
        <Text className="text-text-muted text-xs">
          {formatDateTime(run.started_at)}
        </Text>
      </View>
    </View>
  );
}

// ---------------------------------------------------------------------------
// Amnesia row
// ---------------------------------------------------------------------------

function AmnesiaRow({ item }: { item: Amnesia }) {
  return (
    <View
      className="py-2.5 px-3"
      style={{
        borderBottomWidth: 1,
        borderBottomColor: colors.border.default,
      }}
    >
      <Text className="text-text-primary text-xs" numberOfLines={2}>
        Rule: {item.rule_text}
      </Text>
      <View className="flex-row items-center gap-2 mt-1">
        <Text className="text-text-muted text-xs">
          Tool: {item.tool_name}
        </Text>
        <Text
          className="text-xs"
          style={{
            color:
              item.status === "flagged"
                ? colors.status.warning
                : item.status === "confirmed"
                  ? colors.status.error
                  : colors.text.muted,
          }}
        >
          {item.status}
        </Text>
      </View>
    </View>
  );
}

// ---------------------------------------------------------------------------
// Delusion row
// ---------------------------------------------------------------------------

function DelusionRow({ item }: { item: Delusion }) {
  return (
    <View
      className="py-2.5 px-3"
      style={{
        borderBottomWidth: 1,
        borderBottomColor: colors.border.default,
      }}
    >
      <Text className="text-text-primary text-xs" numberOfLines={2}>
        {item.reason}
      </Text>
      <View className="flex-row items-center gap-2 mt-1">
        <Text className="text-text-muted text-xs">
          Tool: {item.tool_name}
        </Text>
        <Text
          className="text-xs"
          style={{
            color:
              item.status === "flagged"
                ? colors.status.warning
                : item.status === "confirmed"
                  ? colors.status.error
                  : colors.text.muted,
          }}
        >
          {item.status}
        </Text>
      </View>
    </View>
  );
}

// ---------------------------------------------------------------------------
// Task Detail Screen
// ---------------------------------------------------------------------------

export default function TaskDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();

  const { data: task, isLoading } = useTask(id ?? "");
  const { data: runs } = useTaskRuns(id ?? "");
  const { data: amnesia } = useTaskAmnesia(id ?? "");
  const { data: delusions } = useTaskDelusions(id ?? "");

  const startMutation = useStartTask();
  const cancelMutation = useCancelTask();
  const killMutation = useKillTask();
  const deleteMutation = useDeleteTask();

  const isRunning = task?.status === "running";
  const isTerminal = ["completed", "failed", "cancelled"].includes(
    task?.status ?? "",
  );
  const isPending = task?.status === "pending" || task?.status === "scheduled";

  // --- Handlers ---

  const handleStart = () => {
    if (!id) return;
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    startMutation.mutate(
      { id },
      {
        onSuccess: () => {
          Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
        },
        onError: (err) => Alert.alert("Error", err.message),
      },
    );
  };

  const handleRerun = () => {
    if (!id) return;
    Alert.alert("Re-run Task", "Start a new run of this task?", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Re-run",
        onPress: () => {
          Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
          startMutation.mutate(
            { id },
            {
              onError: (err) => Alert.alert("Error", err.message),
            },
          );
        },
      },
    ]);
  };

  const handleCancel = () => {
    if (!id) return;
    Alert.alert("Cancel Task", "Cancel the running task?", [
      { text: "No", style: "cancel" },
      {
        text: "Cancel Task",
        style: "destructive",
        onPress: () => {
          Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Heavy);
          cancelMutation.mutate(id, {
            onError: (err) => Alert.alert("Error", err.message),
          });
        },
      },
    ]);
  };

  const handleKill = () => {
    if (!id) return;
    Alert.alert(
      "Kill Task",
      "Force-kill this task immediately? This cannot be undone.",
      [
        { text: "No", style: "cancel" },
        {
          text: "Kill",
          style: "destructive",
          onPress: () => {
            Haptics.notificationAsync(
              Haptics.NotificationFeedbackType.Error,
            );
            killMutation.mutate(id, {
              onError: (err) => Alert.alert("Error", err.message),
            });
          },
        },
      ],
    );
  };

  const handleDelete = () => {
    if (!id) return;
    Alert.alert(
      "Delete Task",
      `Delete "${task?.title}"? It can be restored from Recall.`,
      [
        { text: "Cancel", style: "cancel" },
        {
          text: "Delete",
          style: "destructive",
          onPress: () => {
            Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Heavy);
            deleteMutation.mutate(id, {
              onSuccess: () => router.back(),
            });
          },
        },
      ],
    );
  };

  // --- Loading / Not found ---

  if (isLoading) {
    return (
      <View className="flex-1 bg-bg-primary items-center justify-center">
        <ActivityIndicator color={colors.accent.default} />
      </View>
    );
  }

  if (!task) {
    return (
      <View className="flex-1 bg-bg-primary items-center justify-center px-4">
        <Text className="text-text-muted text-sm">Task not found</Text>
      </View>
    );
  }

  return (
    <ScrollView
      className="flex-1 bg-bg-primary px-4 pt-4"
      contentContainerStyle={{ paddingBottom: 40 }}
    >
      {/* Header card */}
      <View className="bg-bg-card rounded-card p-4 mb-4">
        <View className="flex-row items-start justify-between mb-2">
          <Text className="text-text-primary text-lg font-bold flex-1 mr-3">
            {task.title}
          </Text>
          <StatusBadge
            status={taskStatusMap[task.status]}
            label={task.status}
          />
        </View>

        {task.description ? (
          <Text className="text-text-secondary text-sm mb-3">
            {task.description}
          </Text>
        ) : null}

        <View
          style={{
            borderTopWidth: 1,
            borderTopColor: colors.border.default,
            paddingTop: 8,
          }}
        >
          <MetaRow label="Marker" value={task.marker} />
          <MetaRow label="Manifest" value={task.manifest_id} />
          <MetaRow label="Schedule" value={task.schedule} />
          <MetaRow
            label="Max Turns"
            value={task.max_turns > 0 ? String(task.max_turns) : "\u2014"}
          />
          <MetaRow label="Agent" value={task.agent} />
          <MetaRow label="Source Node" value={task.source_node} />
          <MetaRow label="Created By" value={task.created_by} />
          <MetaRow label="Run Count" value={String(task.run_count)} />
          <MetaRow label="Last Run" value={formatDateTime(task.last_run_at)} />
          <MetaRow label="Next Run" value={formatDateTime(task.next_run_at)} />
          <MetaRow label="Depends On" value={task.depends_on} />
          <MetaRow
            label="Created"
            value={formatDateTime(task.created_at)}
          />
          <MetaRow
            label="Updated"
            value={formatDateTime(task.updated_at)}
          />
        </View>
      </View>

      {/* Action buttons */}
      <View className="flex-row gap-3 mb-4">
        {/* Start / Re-run */}
        {isPending && (
          <Pressable
            className="flex-1 rounded-lg p-3 items-center active:opacity-70"
            style={{ backgroundColor: `${colors.status.success}22` }}
            onPress={handleStart}
            disabled={startMutation.isPending}
          >
            {startMutation.isPending ? (
              <ActivityIndicator color={colors.status.success} size="small" />
            ) : (
              <Text
                style={{ color: colors.status.success }}
                className="font-semibold text-sm"
              >
                Start
              </Text>
            )}
          </Pressable>
        )}

        {isTerminal && (
          <Pressable
            className="flex-1 rounded-lg p-3 items-center active:opacity-70"
            style={{ backgroundColor: `${colors.status.success}22` }}
            onPress={handleRerun}
            disabled={startMutation.isPending}
          >
            {startMutation.isPending ? (
              <ActivityIndicator color={colors.status.success} size="small" />
            ) : (
              <Text
                style={{ color: colors.status.success }}
                className="font-semibold text-sm"
              >
                Re-run
              </Text>
            )}
          </Pressable>
        )}

        {/* Cancel */}
        {isRunning && (
          <Pressable
            className="flex-1 rounded-lg p-3 items-center active:opacity-70"
            style={{ backgroundColor: `${colors.status.warning}22` }}
            onPress={handleCancel}
            disabled={cancelMutation.isPending}
          >
            {cancelMutation.isPending ? (
              <ActivityIndicator color={colors.status.warning} size="small" />
            ) : (
              <Text
                style={{ color: colors.status.warning }}
                className="font-semibold text-sm"
              >
                Cancel
              </Text>
            )}
          </Pressable>
        )}

        {/* Kill */}
        {isRunning && (
          <Pressable
            className="flex-1 rounded-lg p-3 items-center active:opacity-70"
            style={{ backgroundColor: `${colors.status.error}22` }}
            onPress={handleKill}
            disabled={killMutation.isPending}
          >
            {killMutation.isPending ? (
              <ActivityIndicator color={colors.status.error} size="small" />
            ) : (
              <Text
                style={{ color: colors.status.error }}
                className="font-semibold text-sm"
              >
                Kill
              </Text>
            )}
          </Pressable>
        )}

        {/* Delete — always available when not running */}
        {!isRunning && (
          <Pressable
            className="flex-1 rounded-lg p-3 items-center active:opacity-70"
            style={{ backgroundColor: `${colors.status.error}22` }}
            onPress={handleDelete}
            disabled={deleteMutation.isPending}
          >
            <Text
              style={{ color: colors.status.error }}
              className="font-semibold text-sm"
            >
              {deleteMutation.isPending ? "Deleting..." : "Delete"}
            </Text>
          </Pressable>
        )}
      </View>

      {/* Run History */}
      <Section
        title="Run History"
        count={runs?.length}
        defaultExpanded={(runs?.length ?? 0) > 0}
      >
        {runs && runs.length > 0 ? (
          <View className="bg-bg-card rounded-card overflow-hidden">
            {runs.map((run) => (
              <RunRow key={run.id} run={run} />
            ))}
          </View>
        ) : (
          <View className="bg-bg-card rounded-card p-4 items-center">
            <Text className="text-text-muted text-xs">No runs yet</Text>
          </View>
        )}
      </Section>

      {/* Amnesia Violations */}
      {amnesia && amnesia.length > 0 && (
        <Section
          title="Amnesia Violations"
          count={amnesia.length}
          defaultExpanded={false}
        >
          <View className="bg-bg-card rounded-card overflow-hidden">
            {amnesia.map((a) => (
              <AmnesiaRow key={a.id} item={a} />
            ))}
          </View>
        </Section>
      )}

      {/* Delusions */}
      {delusions && delusions.length > 0 && (
        <Section
          title="Delusions"
          count={delusions.length}
          defaultExpanded={false}
        >
          <View className="bg-bg-card rounded-card overflow-hidden">
            {delusions.map((d) => (
              <DelusionRow key={d.id} item={d} />
            ))}
          </View>
        </Section>
      )}
    </ScrollView>
  );
}
