/**
 * Push notification system for OpenPraxis Mobile.
 *
 * Handles:
 *   - Permission requests and registration
 *   - Notification categories with action buttons (e.g., "Stop Task")
 *   - Typed notification dispatchers for each event type
 *   - Notification tap → deep-link navigation
 *   - User preferences (per-type enable/disable) persisted in SQLite
 */

import * as Notifications from "expo-notifications";
import { router } from "expo-router";
import { Platform } from "react-native";
import { useAppStore } from "./store";
import { configGet, configSet } from "./db";

// ---------------------------------------------------------------------------
// Notification category identifiers
// ---------------------------------------------------------------------------

export const CATEGORY_TASK = "task_event";
export const CATEGORY_AMNESIA = "amnesia_event";
export const CATEGORY_EMERGENCY = "emergency_event";

// Action identifiers
export const ACTION_VIEW = "view";
export const ACTION_STOP = "stop_task";

// ---------------------------------------------------------------------------
// Foreground notification display configuration
// ---------------------------------------------------------------------------

Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowAlert: true,
    shouldShowBanner: true,
    shouldShowList: true,
    shouldPlaySound: true,
    shouldSetBadge: false,
  }),
});

// ---------------------------------------------------------------------------
// Notification categories (with action buttons)
// ---------------------------------------------------------------------------

/** Set up notification categories with actionable buttons. */
export async function setupNotificationCategories(): Promise<void> {
  await Notifications.setNotificationCategoryAsync(CATEGORY_TASK, [
    {
      identifier: ACTION_VIEW,
      buttonTitle: "View",
      options: { opensAppToForeground: true },
    },
  ]);

  await Notifications.setNotificationCategoryAsync(CATEGORY_AMNESIA, [
    {
      identifier: ACTION_VIEW,
      buttonTitle: "View Task",
      options: { opensAppToForeground: true },
    },
  ]);

  await Notifications.setNotificationCategoryAsync(CATEGORY_EMERGENCY, [
    {
      identifier: ACTION_STOP,
      buttonTitle: "Stop Task",
      options: { opensAppToForeground: true, isDestructive: true },
    },
    {
      identifier: ACTION_VIEW,
      buttonTitle: "View",
      options: { opensAppToForeground: true },
    },
  ]);
}

// ---------------------------------------------------------------------------
// Permission + Registration
// ---------------------------------------------------------------------------

/** Request notification permissions and set up categories. */
export async function registerForPushNotifications(): Promise<string | null> {
  const { status: existingStatus } =
    await Notifications.getPermissionsAsync();
  let finalStatus = existingStatus;

  if (existingStatus !== "granted") {
    const { status } = await Notifications.requestPermissionsAsync();
    finalStatus = status;
  }

  if (finalStatus !== "granted") {
    return null;
  }

  // Set up categories after permission is granted
  await setupNotificationCategories();

  // For local notifications we don't need an Expo push token,
  // but we track permission status
  const token = "local-notifications-enabled";
  useAppStore.getState().setPushToken(token);
  return token;
}

// ---------------------------------------------------------------------------
// Notification data payload (for deep-link navigation on tap)
// ---------------------------------------------------------------------------

export interface NotificationData {
  [key: string]: unknown;
  route: string;
  taskId?: string;
  amnesiaId?: number;
  delusionId?: number;
}

// ---------------------------------------------------------------------------
// Typed notification dispatchers
// ---------------------------------------------------------------------------

/** Push notification when a task completes successfully. */
export async function notifyTaskCompleted(
  taskId: string,
  title?: string,
): Promise<void> {
  if (!(await isEnabled("task_completed"))) return;

  const label = title || taskId.slice(0, 8);
  await Notifications.scheduleNotificationAsync({
    content: {
      title: "Task Completed",
      body: `${label} finished successfully.`,
      categoryIdentifier: CATEGORY_TASK,
      data: { route: `/tasks/${taskId}`, taskId } as NotificationData,
      sound: "default",
    },
    trigger: null,
  });
}

/** Push notification when a task fails. */
export async function notifyTaskFailed(
  taskId: string,
  error: string,
): Promise<void> {
  if (!(await isEnabled("task_failed"))) return;

  await Notifications.scheduleNotificationAsync({
    content: {
      title: "Task Failed",
      body: `${taskId.slice(0, 8)}: ${error}`,
      categoryIdentifier: CATEGORY_TASK,
      data: { route: `/tasks/${taskId}`, taskId } as NotificationData,
      sound: "default",
    },
    trigger: null,
  });
}

/** Push notification when an amnesia violation is detected. */
export async function notifyAmnesiaViolation(
  taskId: string,
  ruleText: string,
  amnesiaId: number,
): Promise<void> {
  if (!(await isEnabled("amnesia"))) return;

  await Notifications.scheduleNotificationAsync({
    content: {
      title: "Amnesia Violation",
      body: `Rule broken: ${ruleText}`,
      categoryIdentifier: CATEGORY_AMNESIA,
      data: {
        route: `/tasks/${taskId}`,
        taskId,
        amnesiaId,
      } as NotificationData,
      sound: "default",
    },
    trigger: null,
  });
}

/** Push notification for delusion (agent going off-spec). Emergency level. */
export async function notifyDelusionDetected(
  taskId: string,
  reason: string,
  delusionId: number,
): Promise<void> {
  if (!(await isEnabled("emergency"))) return;

  await Notifications.scheduleNotificationAsync({
    content: {
      title: "Agent Off-Spec",
      body: reason,
      categoryIdentifier: CATEGORY_EMERGENCY,
      data: {
        route: `/tasks/${taskId}`,
        taskId,
        delusionId,
      } as NotificationData,
      sound: "default",
    },
    trigger: null,
  });
}

/** Push notification when a peer joins or leaves. */
export async function notifyPeerEvent(
  event: "joined" | "left",
  nodeId: string,
): Promise<void> {
  if (!(await isEnabled("peer_events"))) return;

  await Notifications.scheduleNotificationAsync({
    content: {
      title: event === "joined" ? "Peer Joined" : "Peer Left",
      body: `${nodeId} ${event === "joined" ? "connected" : "disconnected"}.`,
      data: { route: "/settings" } as NotificationData,
    },
    trigger: null,
  });
}

// ---------------------------------------------------------------------------
// Notification response handler (tap → navigate)
// ---------------------------------------------------------------------------

/**
 * Handle user tapping a notification or pressing an action button.
 * Call this once from the root layout to wire up navigation.
 *
 * Returns an unsubscribe function.
 */
export function setupNotificationResponseHandler(
  onStopTask?: (taskId: string) => void,
): () => void {
  const subscription = Notifications.addNotificationResponseReceivedListener(
    (response) => {
      const data = response.notification.request.content
        .data as unknown as NotificationData | undefined;

      const actionId = response.actionIdentifier;

      // Handle "Stop Task" action button
      if (
        actionId === ACTION_STOP &&
        data?.taskId &&
        onStopTask
      ) {
        onStopTask(data.taskId);
        return;
      }

      // Default: navigate to the relevant screen
      if (data?.route) {
        // Small delay to ensure the app is in the foreground
        setTimeout(() => {
          router.push(data.route as never);
        }, 100);
      }
    },
  );

  return () => subscription.remove();
}

/**
 * Check if the app was launched by a notification tap (cold start).
 * If so, navigate to the appropriate screen.
 */
export async function handleInitialNotification(): Promise<void> {
  const response = await Notifications.getLastNotificationResponseAsync();
  if (!response) return;

  const data = response.notification.request.content
    .data as NotificationData | undefined;

  if (data?.route) {
    setTimeout(() => {
      router.push(data.route as never);
    }, 500);
  }
}

// ---------------------------------------------------------------------------
// Notification preferences (persisted in SQLite peer_config)
// ---------------------------------------------------------------------------

export type NotificationPreferenceKey =
  | "task_completed"
  | "task_failed"
  | "amnesia"
  | "emergency"
  | "peer_events";

const PREF_PREFIX = "notif_pref_";

const DEFAULT_PREFERENCES: Record<NotificationPreferenceKey, boolean> = {
  task_completed: true,
  task_failed: true,
  amnesia: true,
  emergency: true,
  peer_events: true,
};

/** Check if a notification type is enabled. */
export async function isEnabled(
  key: NotificationPreferenceKey,
): Promise<boolean> {
  try {
    const val = await configGet(`${PREF_PREFIX}${key}`);
    if (val === null) return DEFAULT_PREFERENCES[key];
    return val === "1";
  } catch {
    return DEFAULT_PREFERENCES[key];
  }
}

/** Set whether a notification type is enabled. */
export async function setEnabled(
  key: NotificationPreferenceKey,
  enabled: boolean,
): Promise<void> {
  await configSet(`${PREF_PREFIX}${key}`, enabled ? "1" : "0");
}

/** Get all notification preferences. */
export async function getAllPreferences(): Promise<
  Record<NotificationPreferenceKey, boolean>
> {
  const keys: NotificationPreferenceKey[] = [
    "task_completed",
    "task_failed",
    "amnesia",
    "emergency",
    "peer_events",
  ];

  const result = { ...DEFAULT_PREFERENCES };

  for (const key of keys) {
    result[key] = await isEnabled(key);
  }

  return result;
}

// ---------------------------------------------------------------------------
// Legacy — kept for backward compatibility with existing callers
// ---------------------------------------------------------------------------

/** Show a local notification (simple fire-and-forget). */
export async function showLocalNotification(
  title: string,
  body: string,
): Promise<void> {
  await Notifications.scheduleNotificationAsync({
    content: { title, body },
    trigger: null,
  });
}
