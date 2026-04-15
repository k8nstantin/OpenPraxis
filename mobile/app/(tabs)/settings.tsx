import { useState } from "react";
import {
  View,
  Text,
  TextInput,
  Pressable,
  ScrollView,
  ActivityIndicator,
  Alert,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import * as Haptics from "expo-haptics";
import { useAppStore, type ConnectionStatus } from "@/lib/store";
import {
  useProfile,
  useUpdateProfile,
  useSyncStatus,
  useTriggerSync,
  useNotificationPreferences,
  useToggleNotificationPref,
} from "@/lib/hooks";
import { colors } from "@/lib/theme";
import type { NotificationPreferenceKey } from "@/lib/notifications";

const STATUS_COLORS: Record<ConnectionStatus, string> = {
  connected: colors.status.success,
  connecting: colors.status.warning,
  disconnected: colors.text.muted,
};

const STATUS_LABELS: Record<ConnectionStatus, string> = {
  connected: "Connected",
  connecting: "Connecting...",
  disconnected: "Disconnected",
};

export default function SettingsScreen() {
  const {
    peerHost,
    setPeerHost,
    connectionStatus,
    connected,
    peerUuid,
    serverStatus,
    lastError,
    connect,
    disconnect,
    syncPending,
    syncInProgress,
    lastSyncAt,
    lastSyncResult,
    syncError,
  } = useAppStore();

  const [hostInput, setHostInput] = useState(peerHost);
  const [testing, setTesting] = useState(false);

  // Profile data from server
  const { data: profile } = useProfile();
  const updateProfile = useUpdateProfile();

  // Sync status
  const { data: syncStatus } = useSyncStatus();
  const triggerSync = useTriggerSync();

  // Notification preferences
  const { data: notifPrefs } = useNotificationPreferences();
  const toggleNotifPref = useToggleNotificationPref();

  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");
  const [avatar, setAvatar] = useState("");
  const [profileDirty, setProfileDirty] = useState(false);

  // Sync profile fields when data arrives
  if (
    profile &&
    !profileDirty &&
    displayName === "" &&
    profile.display_name !== ""
  ) {
    setDisplayName(profile.display_name);
    setEmail(profile.email);
    setAvatar(profile.avatar);
  }

  const handleTestConnection = async () => {
    setTesting(true);
    setPeerHost(hostInput);

    try {
      await connect();
      Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    } catch {
      Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
    } finally {
      setTesting(false);
    }
  };

  const handleDisconnect = () => {
    disconnect();
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
  };

  const handleSaveProfile = async () => {
    try {
      await updateProfile.mutateAsync({
        display_name: displayName,
        email,
        avatar,
      });
      setProfileDirty(false);
      Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    } catch {
      Alert.alert("Error", "Failed to save profile");
    }
  };

  return (
    <SafeAreaView className="flex-1 bg-bg-primary">
      <ScrollView className="flex-1 px-4 pt-4">
        <Text className="text-text-primary text-2xl font-bold mb-6">
          Settings
        </Text>

        {/* Connection */}
        <View className="bg-bg-card rounded-card p-4 mb-4">
          <Text className="text-text-secondary text-sm font-semibold mb-3 uppercase tracking-wider">
            Connection
          </Text>
          <Text className="text-text-muted text-xs mb-2">
            OpenLoom server address (IP:port)
          </Text>
          <TextInput
            className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm"
            style={{ borderColor: colors.border.default, borderWidth: 1 }}
            value={hostInput}
            onChangeText={setHostInput}
            placeholder="192.168.1.100:8765"
            placeholderTextColor={colors.text.muted}
            autoCapitalize="none"
            autoCorrect={false}
            keyboardType="url"
          />

          {/* Status indicator */}
          <View className="flex-row items-center gap-2 mt-3">
            <View
              style={{
                width: 8,
                height: 8,
                borderRadius: 4,
                backgroundColor: STATUS_COLORS[connectionStatus],
              }}
            />
            <Text className="text-text-muted text-xs">
              {STATUS_LABELS[connectionStatus]}
            </Text>
            {serverStatus && connected && (
              <Text className="text-text-muted text-xs ml-auto">
                {serverStatus.node} / {serverStatus.uptime}
              </Text>
            )}
          </View>

          {/* Error message */}
          {lastError && (
            <Text className="text-xs mt-2" style={{ color: colors.status.error }}>
              {lastError}
            </Text>
          )}

          {/* Connect / Disconnect buttons */}
          <View className="flex-row gap-3 mt-4">
            {!connected ? (
              <Pressable
                className="flex-1 rounded-lg py-2.5 items-center"
                style={{ backgroundColor: colors.accent.default }}
                onPress={handleTestConnection}
                disabled={testing || !hostInput.trim()}
              >
                {testing ? (
                  <ActivityIndicator size="small" color="#fff" />
                ) : (
                  <Text className="text-white text-sm font-semibold">
                    Connect
                  </Text>
                )}
              </Pressable>
            ) : (
              <Pressable
                className="flex-1 rounded-lg py-2.5 items-center"
                style={{
                  backgroundColor: "rgba(255,255,255,0.08)",
                  borderColor: colors.status.error,
                  borderWidth: 1,
                }}
                onPress={handleDisconnect}
              >
                <Text
                  className="text-sm font-semibold"
                  style={{ color: colors.status.error }}
                >
                  Disconnect
                </Text>
              </Pressable>
            )}
          </View>
        </View>

        {/* Peer Identity */}
        <View className="bg-bg-card rounded-card p-4 mb-4">
          <Text className="text-text-secondary text-sm font-semibold mb-3 uppercase tracking-wider">
            Peer Identity
          </Text>
          {peerUuid ? (
            <>
              <Text className="text-text-muted text-xs mb-1">Device UUID</Text>
              <Text
                className="text-text-primary text-xs font-mono"
                selectable
              >
                {peerUuid}
              </Text>
            </>
          ) : (
            <Text className="text-text-muted text-xs">
              UUID will be generated on first connection.
            </Text>
          )}
        </View>

        {/* P2P Sync Status */}
        <View className="bg-bg-card rounded-card p-4 mb-4">
          <Text className="text-text-secondary text-sm font-semibold mb-3 uppercase tracking-wider">
            P2P Sync
          </Text>

          {/* Sync state indicator */}
          <View className="flex-row items-center gap-2 mb-3">
            <View
              style={{
                width: 8,
                height: 8,
                borderRadius: 4,
                backgroundColor: syncInProgress
                  ? colors.status.warning
                  : syncPending > 0
                    ? colors.accent.default
                    : connected
                      ? colors.status.success
                      : colors.text.muted,
              }}
            />
            <Text className="text-text-muted text-xs">
              {syncInProgress
                ? "Syncing..."
                : syncPending > 0
                  ? `${syncPending} pending change${syncPending !== 1 ? "s" : ""}`
                  : connected
                    ? "In sync"
                    : "Offline"}
            </Text>
          </View>

          {/* Last sync time */}
          {lastSyncAt && (
            <InfoRow
              label="Last sync"
              value={new Date(lastSyncAt).toLocaleTimeString()}
            />
          )}

          {/* Last sync result */}
          {lastSyncResult && (
            <>
              <InfoRow
                label="Last pull"
                value={`${lastSyncResult.pulled} items`}
              />
              <InfoRow
                label="Last push"
                value={`${lastSyncResult.pushed} items`}
              />
              {lastSyncResult.conflicts > 0 && (
                <InfoRow
                  label="Conflicts resolved"
                  value={String(lastSyncResult.conflicts)}
                />
              )}
            </>
          )}

          {/* Entity counts from sync status */}
          {syncStatus && (
            <View className="mt-2 pt-2" style={{ borderTopColor: colors.border.default, borderTopWidth: 1 }}>
              <InfoRow
                label="Manifests"
                value={`${syncStatus.manifests.count} synced`}
              />
              <InfoRow
                label="Ideas"
                value={`${syncStatus.ideas.count} synced`}
              />
              <InfoRow
                label="Visceral rules"
                value={`${syncStatus.visceral.count} synced`}
              />
            </View>
          )}

          {/* Sync error */}
          {syncError && (
            <Text
              className="text-xs mt-2"
              style={{ color: colors.status.error }}
            >
              {syncError}
            </Text>
          )}

          {/* Manual sync button */}
          <Pressable
            className="rounded-lg py-2.5 items-center mt-3"
            style={{
              backgroundColor: syncInProgress
                ? "rgba(255,255,255,0.05)"
                : colors.accent.default,
            }}
            onPress={() => {
              triggerSync.mutate();
              Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
            }}
            disabled={syncInProgress || triggerSync.isPending}
          >
            {syncInProgress || triggerSync.isPending ? (
              <ActivityIndicator size="small" color="#fff" />
            ) : (
              <Text className="text-white text-sm font-semibold">
                Sync Now
              </Text>
            )}
          </Pressable>
        </View>

        {/* Notification Preferences */}
        <View className="bg-bg-card rounded-card p-4 mb-4">
          <Text className="text-text-secondary text-sm font-semibold mb-3 uppercase tracking-wider">
            Notifications
          </Text>
          <Text className="text-text-muted text-xs mb-3">
            Control which events trigger push notifications.
          </Text>

          <NotifToggle
            label="Task Completed"
            subtitle="When a task finishes successfully"
            prefKey="task_completed"
            prefs={notifPrefs}
            onToggle={toggleNotifPref.mutate}
          />
          <NotifToggle
            label="Task Failed"
            subtitle="When a task encounters an error"
            prefKey="task_failed"
            prefs={notifPrefs}
            onToggle={toggleNotifPref.mutate}
          />
          <NotifToggle
            label="Amnesia Violation"
            subtitle="When an agent breaks a visceral rule"
            prefKey="amnesia"
            prefs={notifPrefs}
            onToggle={toggleNotifPref.mutate}
          />
          <NotifToggle
            label="Agent Off-Spec (Emergency)"
            subtitle="When an agent deviates from its manifest"
            prefKey="emergency"
            prefs={notifPrefs}
            onToggle={toggleNotifPref.mutate}
          />
          <NotifToggle
            label="Peer Events"
            subtitle="When peers connect or disconnect"
            prefKey="peer_events"
            prefs={notifPrefs}
            onToggle={toggleNotifPref.mutate}
          />
        </View>

        {/* Server Profile (only when connected) */}
        {connected && (
          <View className="bg-bg-card rounded-card p-4 mb-4">
            <Text className="text-text-secondary text-sm font-semibold mb-3 uppercase tracking-wider">
              Server Profile
            </Text>

            <Text className="text-text-muted text-xs mb-1">Display Name</Text>
            <TextInput
              className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm mb-3"
              style={{ borderColor: colors.border.default, borderWidth: 1 }}
              value={displayName}
              onChangeText={(v) => {
                setDisplayName(v);
                setProfileDirty(true);
              }}
              placeholder="Your name"
              placeholderTextColor={colors.text.muted}
            />

            <Text className="text-text-muted text-xs mb-1">Email</Text>
            <TextInput
              className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm mb-3"
              style={{ borderColor: colors.border.default, borderWidth: 1 }}
              value={email}
              onChangeText={(v) => {
                setEmail(v);
                setProfileDirty(true);
              }}
              placeholder="email@example.com"
              placeholderTextColor={colors.text.muted}
              keyboardType="email-address"
              autoCapitalize="none"
            />

            <Text className="text-text-muted text-xs mb-1">Avatar Emoji</Text>
            <TextInput
              className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm mb-3"
              style={{ borderColor: colors.border.default, borderWidth: 1 }}
              value={avatar}
              onChangeText={(v) => {
                setAvatar(v);
                setProfileDirty(true);
              }}
              placeholder="e.g. laptop"
              placeholderTextColor={colors.text.muted}
            />

            {profileDirty && (
              <Pressable
                className="rounded-lg py-2.5 items-center mt-1"
                style={{ backgroundColor: colors.accent.default }}
                onPress={handleSaveProfile}
                disabled={updateProfile.isPending}
              >
                {updateProfile.isPending ? (
                  <ActivityIndicator size="small" color="#fff" />
                ) : (
                  <Text className="text-white text-sm font-semibold">
                    Save Profile
                  </Text>
                )}
              </Pressable>
            )}
          </View>
        )}

        {/* Server Info (only when connected) */}
        {connected && serverStatus && (
          <View className="bg-bg-card rounded-card p-4 mb-4">
            <Text className="text-text-secondary text-sm font-semibold mb-3 uppercase tracking-wider">
              Server Info
            </Text>
            <InfoRow label="Node" value={serverStatus.node} />
            <InfoRow label="Uptime" value={serverStatus.uptime} />
            <InfoRow label="Memories" value={String(serverStatus.memories)} />
            <InfoRow label="Sessions" value={String(serverStatus.sessions)} />
            <InfoRow label="Peers" value={String(serverStatus.peers)} />
            <InfoRow label="Embedding" value={serverStatus.embedding} />
          </View>
        )}

        {/* About */}
        <View className="bg-bg-card rounded-card p-4 mb-8">
          <Text className="text-text-secondary text-sm font-semibold mb-2 uppercase tracking-wider">
            About
          </Text>
          <Text className="text-text-muted text-xs">
            OpenLoom Mobile v1.0.0
          </Text>
          <Text className="text-text-muted text-xs mt-1">
            Phone as a Peer Node
          </Text>
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <View className="flex-row justify-between py-1">
      <Text className="text-text-muted text-xs">{label}</Text>
      <Text className="text-text-primary text-xs">{value}</Text>
    </View>
  );
}

function NotifToggle({
  label,
  subtitle,
  prefKey,
  prefs,
  onToggle,
}: {
  label: string;
  subtitle: string;
  prefKey: NotificationPreferenceKey;
  prefs: Record<NotificationPreferenceKey, boolean> | undefined;
  onToggle: (args: { key: NotificationPreferenceKey; enabled: boolean }) => void;
}) {
  const enabled = prefs?.[prefKey] ?? true;

  return (
    <Pressable
      className="flex-row items-center py-2.5"
      style={{
        borderBottomWidth: 1,
        borderBottomColor: colors.border.default,
      }}
      onPress={() => {
        Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
        onToggle({ key: prefKey, enabled: !enabled });
      }}
    >
      <View className="flex-1 mr-3">
        <Text className="text-text-primary text-sm">{label}</Text>
        <Text className="text-text-muted text-xs mt-0.5">{subtitle}</Text>
      </View>
      <View
        className="w-11 h-6 rounded-full justify-center px-0.5"
        style={{
          backgroundColor: enabled
            ? colors.accent.default
            : "rgba(255,255,255,0.1)",
        }}
      >
        <View
          className="w-5 h-5 rounded-full"
          style={{
            backgroundColor: "#fff",
            alignSelf: enabled ? "flex-end" : "flex-start",
          }}
        />
      </View>
    </Pressable>
  );
}
