import "../global.css";
import { useEffect } from "react";
import { AppState as RNAppState } from "react-native";
import { Stack } from "expo-router";
import { StatusBar } from "expo-status-bar";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { GestureHandlerRootView } from "react-native-gesture-handler";
import { colors } from "@/lib/theme";
import { useAppStore } from "@/lib/store";
import { openDb } from "@/lib/db";
import {
  registerForPushNotifications,
  setupNotificationResponseHandler,
  handleInitialNotification,
  notifyTaskCompleted,
  notifyTaskFailed,
  notifyAmnesiaViolation,
  notifyDelusionDetected,
  notifyPeerEvent,
} from "@/lib/notifications";
import { api } from "@/lib/api";
import { syncAll, stopPeriodicSync, startPeriodicSync } from "@/lib/p2pSync";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5_000,
      retry: 1,
    },
  },
});

export default function RootLayout() {
  useEffect(() => {
    // Initialize: open DB, hydrate store, register notifications, auto-connect
    async function init() {
      await openDb();
      await useAppStore.getState().hydrate();
      await registerForPushNotifications();

      // Handle cold-start from notification tap
      await handleInitialNotification();

      // Auto-connect if we have a stored host
      const { peerHost } = useAppStore.getState();
      if (peerHost) {
        try {
          await useAppStore.getState().connect();
        } catch {
          // Silent fail on auto-connect — user can connect manually
        }
      }

      // Wire WebSocket events to typed notifications
      const { socket } = useAppStore.getState();
      if (socket) {
        socket.onTaskCompleted((data) => {
          notifyTaskCompleted(data.task_id);
          queryClient.invalidateQueries({ queryKey: ["tasks"] });
        });

        socket.onTaskFailed((data) => {
          notifyTaskFailed(data.task_id, data.error);
          queryClient.invalidateQueries({ queryKey: ["tasks"] });
        });

        socket.onMemoryStored(() => {
          queryClient.invalidateQueries({ queryKey: ["memories"] });
        });

        socket.onPeerJoined((data) => {
          notifyPeerEvent("joined", data.node_id);
          queryClient.invalidateQueries({ queryKey: ["peers"] });
          queryClient.invalidateQueries({ queryKey: ["status"] });
        });

        socket.onPeerLeft((data) => {
          notifyPeerEvent("left", data.node_id);
          queryClient.invalidateQueries({ queryKey: ["peers"] });
          queryClient.invalidateQueries({ queryKey: ["status"] });
        });

        // Amnesia violation — visceral rule was broken
        socket.onAmnesiaDetected((data) => {
          notifyAmnesiaViolation(
            data.task_id,
            data.rule_text,
            data.amnesia_id,
          );
          queryClient.invalidateQueries({ queryKey: ["tasks"] });
          queryClient.invalidateQueries({ queryKey: ["amnesia"] });
        });

        // Delusion detected — agent going off-spec (emergency)
        socket.onDelusionDetected((data) => {
          notifyDelusionDetected(
            data.task_id,
            data.reason,
            data.delusion_id,
          );
          queryClient.invalidateQueries({ queryKey: ["tasks"] });
          queryClient.invalidateQueries({ queryKey: ["delusions"] });
        });
      }
    }

    init();

    // Wire notification tap → navigation + "Stop Task" action
    const unsubNotifResponse = setupNotificationResponseHandler(
      (taskId: string) => {
        // "Stop Task" action pressed from notification
        api.killTask(taskId).then(() => {
          queryClient.invalidateQueries({ queryKey: ["tasks"] });
        });
      },
    );

    // Re-sync when app comes back to foreground
    const appStateSubscription = RNAppState.addEventListener(
      "change",
      (nextState) => {
        if (nextState === "active" && useAppStore.getState().connected) {
          syncAll().catch(() => {});
          startPeriodicSync();
        } else if (nextState === "background") {
          stopPeriodicSync();
        }
      },
    );

    // Cleanup on unmount
    return () => {
      appStateSubscription.remove();
      unsubNotifResponse();
      stopPeriodicSync();
      const { socket } = useAppStore.getState();
      if (socket) socket.disconnect();
    };
  }, []);

  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <QueryClientProvider client={queryClient}>
        <StatusBar style="light" />
        <Stack
          screenOptions={{
            headerShown: false,
            contentStyle: { backgroundColor: colors.bg.primary },
            animation: "fade",
          }}
        />
      </QueryClientProvider>
    </GestureHandlerRootView>
  );
}
