import { useCallback, useState } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  RefreshControl,
  Alert,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useQueryClient } from "@tanstack/react-query";
import * as Haptics from "expo-haptics";
import { colors } from "@/lib/theme";
import { useAppStore } from "@/lib/store";
import { useStatus, useRunningTasks, useKillTask, queryKeys } from "@/lib/hooks";
import { api } from "@/lib/api";
import { RunningTasks } from "@/components/RunningTasks";
import { PulsingDot } from "@/components/PulsingDot";
import type { ConnectionStatus } from "@/lib/store";
import { syncAll } from "@/lib/p2pSync";

// ---------------------------------------------------------------------------
// Metric Card
// ---------------------------------------------------------------------------

function MetricCard({
  icon,
  label,
  value,
}: {
  icon: string;
  label: string;
  value: string | number;
}) {
  return (
    <View className="bg-bg-card rounded-card p-4 flex-1 min-w-[45%]">
      <Text className="text-2xl mb-1">{icon}</Text>
      <Text className="text-text-primary text-xl font-bold">
        {String(value)}
      </Text>
      <Text className="text-text-muted text-xs mt-0.5">{label}</Text>
    </View>
  );
}

// ---------------------------------------------------------------------------
// Connection indicator
// ---------------------------------------------------------------------------

const statusColorMap: Record<ConnectionStatus, string> = {
  connected: colors.status.success,
  connecting: colors.status.warning,
  disconnected: colors.text.muted,
};

const statusLabelMap: Record<ConnectionStatus, string> = {
  connected: "Connected",
  connecting: "Connecting...",
  disconnected: "Disconnected",
};

function ConnectionDot({ status }: { status: ConnectionStatus }) {
  if (status === "connected") {
    return <PulsingDot color={statusColorMap.connected} size={8} />;
  }
  return (
    <View
      style={{
        width: 8,
        height: 8,
        borderRadius: 4,
        backgroundColor: statusColorMap[status],
      }}
    />
  );
}

// ---------------------------------------------------------------------------
// Overview Screen
// ---------------------------------------------------------------------------

export default function OverviewScreen() {
  const queryClient = useQueryClient();
  const connectionStatus = useAppStore((s) => s.connectionStatus);
  const connected = useAppStore((s) => s.connected);
  const serverStatus = useAppStore((s) => s.serverStatus);
  const connect = useAppStore((s) => s.connect);
  const syncPending = useAppStore((s) => s.syncPending);
  const syncInProgress = useAppStore((s) => s.syncInProgress);

  const { data: status } = useStatus();
  const { data: runningTasks } = useRunningTasks();
  const killMutation = useKillTask();

  const [refreshing, setRefreshing] = useState(false);
  const [killingId, setKillingId] = useState<string | null>(null);
  const [stoppingAll, setStoppingAll] = useState(false);

  // Pull-to-refresh: re-fetch status + running tasks + trigger P2P sync
  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    try {
      if (!connected) {
        await connect();
      }
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.status }),
        queryClient.invalidateQueries({ queryKey: queryKeys.runningTasks }),
        connected ? syncAll().catch(() => {}) : Promise.resolve(),
      ]);
    } catch {
      // swallow — UI shows disconnected state
    } finally {
      setRefreshing(false);
    }
  }, [connected, connect, queryClient]);

  // Kill a single task
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

  // Emergency stop all
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
              queryClient.invalidateQueries({ queryKey: queryKeys.runningTasks });
              queryClient.invalidateQueries({ queryKey: queryKeys.tasks });
            } finally {
              setStoppingAll(false);
            }
          },
        },
      ],
    );
  }, [runningTasks, queryClient]);

  // Use live status or fall back to server status snapshot from connect()
  const liveStatus = status ?? serverStatus;
  const runningCount = runningTasks?.length ?? 0;

  return (
    <SafeAreaView className="flex-1 bg-bg-primary">
      <ScrollView
        className="flex-1 px-4"
        contentContainerStyle={{ paddingBottom: 32 }}
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            onRefresh={onRefresh}
            tintColor={colors.text.muted}
          />
        }
      >
        {/* Header */}
        <View className="flex-row items-center justify-between pt-4 pb-6">
          <View>
            <Text className="text-text-primary text-2xl font-bold">
              OpenLoom
            </Text>
            <Text className="text-text-muted text-sm">
              {liveStatus?.display_name || "Mobile Peer"}
            </Text>
          </View>
          <View className="items-end gap-1">
            <View className="flex-row items-center gap-2">
              <ConnectionDot status={connectionStatus} />
              <Text
                style={{ color: statusColorMap[connectionStatus] }}
                className="text-xs"
              >
                {statusLabelMap[connectionStatus]}
              </Text>
            </View>
            {connected && (syncPending > 0 || syncInProgress) && (
              <Text
                className="text-xs"
                style={{ color: syncInProgress ? colors.status.warning : colors.accent.default }}
              >
                {syncInProgress ? "Syncing..." : `${syncPending} pending`}
              </Text>
            )}
          </View>
        </View>

        {/* Metrics Grid */}
        <View className="flex-row flex-wrap gap-3 mb-6">
          <MetricCard
            icon={"\u25B6"}
            label="Running"
            value={connected ? runningCount : "--"}
          />
          <MetricCard
            icon={"\u25A1"}
            label="Memories"
            value={connected ? (liveStatus?.memories ?? 0) : "--"}
          />
          <MetricCard
            icon={"\u25C7"}
            label="Nodes"
            value={connected ? (liveStatus?.peers ?? 0) : "--"}
          />
          <MetricCard
            icon={"\u2631"}
            label="Sessions"
            value={connected ? (liveStatus?.sessions ?? 0) : "--"}
          />
        </View>

        {/* Running Tasks */}
        <View className="mb-6">
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
          {connected ? (
            <RunningTasks
              tasks={runningTasks ?? []}
              onKill={handleKill}
              killing={killingId}
            />
          ) : (
            <View className="bg-bg-card rounded-card p-6 items-center">
              <Text className="text-text-muted text-sm">
                Connect to a peer to see running tasks
              </Text>
            </View>
          )}
        </View>

        {/* Emergency Stop All */}
        <Pressable
          className="rounded-card p-4 items-center active:opacity-70"
          style={{
            backgroundColor: runningCount > 0
              ? `${colors.status.error}33`
              : `${colors.status.error}11`,
            opacity: runningCount > 0 ? 1 : 0.5,
          }}
          onPress={handleStopAll}
          disabled={runningCount === 0 || stoppingAll}
        >
          <Text
            style={{ color: colors.status.error }}
            className="text-base font-bold"
          >
            {stoppingAll
              ? "Stopping All..."
              : `Emergency Stop All${runningCount > 0 ? ` (${runningCount})` : ""}`}
          </Text>
        </Pressable>

        {/* Uptime */}
        {connected && liveStatus?.uptime && (
          <Text className="text-text-muted text-xs text-center mt-4">
            Server uptime: {liveStatus.uptime}
          </Text>
        )}
      </ScrollView>
    </SafeAreaView>
  );
}
