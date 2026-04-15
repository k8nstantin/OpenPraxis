import { useState, useMemo } from "react";
import {
  View,
  Text,
  TextInput,
  Pressable,
  ScrollView,
  Alert,
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
  Modal,
  FlatList,
} from "react-native";
import { router } from "expo-router";
import * as Haptics from "expo-haptics";
import { colors } from "@/lib/theme";
import {
  useManifests,
  useCreateTask,
  useSettingsAgents,
} from "@/lib/hooks";
import { SchedulePicker } from "@/components/SchedulePicker";
import type { Manifest, TaskCreateRequest } from "@/lib/types";

// ---------------------------------------------------------------------------
// Form label (reused pattern from manifests/create.tsx)
// ---------------------------------------------------------------------------

function Label({ text, required }: { text: string; required?: boolean }) {
  return (
    <Text className="text-text-muted text-xs mb-2">
      {text}
      {required && <Text style={{ color: colors.status.error }}> *</Text>}
    </Text>
  );
}

// ---------------------------------------------------------------------------
// Max Turns Selector — quick presets + custom input
// ---------------------------------------------------------------------------

const TURNS_PRESETS = [10, 25, 50, 100, 200];

function MaxTurnsPicker({
  value,
  onChange,
}: {
  value: number;
  onChange: (n: number) => void;
}) {
  const [customText, setCustomText] = useState("");
  const isPreset = TURNS_PRESETS.includes(value);

  const handleCustom = (text: string) => {
    setCustomText(text);
    const n = parseInt(text, 10);
    if (!isNaN(n) && n > 0) onChange(n);
  };

  return (
    <View>
      <View className="flex-row flex-wrap gap-2 mb-2">
        {TURNS_PRESETS.map((n) => {
          const active = value === n && isPreset;
          return (
            <Pressable
              key={n}
              onPress={() => {
                onChange(n);
                setCustomText("");
              }}
              className="px-3.5 py-1.5 rounded-full active:opacity-70"
              style={{
                backgroundColor: active
                  ? colors.accent.default
                  : `${colors.accent.default}15`,
              }}
            >
              <Text
                className="text-xs font-semibold"
                style={{ color: active ? "#fff" : colors.accent.hover }}
              >
                {n}
              </Text>
            </Pressable>
          );
        })}
      </View>
      <TextInput
        className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm"
        style={{ borderColor: colors.border.default, borderWidth: 1 }}
        placeholder="Custom max turns"
        placeholderTextColor={colors.text.muted}
        value={!isPreset && value > 0 ? String(value) : customText}
        onChangeText={handleCustom}
        keyboardType="number-pad"
      />
    </View>
  );
}

// ---------------------------------------------------------------------------
// Searchable Manifest Picker Modal
// ---------------------------------------------------------------------------

function ManifestPickerModal({
  visible,
  manifests,
  loading,
  onSelect,
  onClose,
}: {
  visible: boolean;
  manifests: Manifest[];
  loading: boolean;
  onSelect: (m: Manifest) => void;
  onClose: () => void;
}) {
  const [search, setSearch] = useState("");

  const filtered = useMemo(() => {
    if (!search.trim()) return manifests;
    const q = search.toLowerCase();
    return manifests.filter(
      (m) =>
        m.title.toLowerCase().includes(q) ||
        m.marker.toLowerCase().includes(q) ||
        m.tags.some((t) => t.toLowerCase().includes(q)),
    );
  }, [manifests, search]);

  return (
    <Modal
      visible={visible}
      animationType="slide"
      presentationStyle="pageSheet"
      onRequestClose={onClose}
    >
      <View className="flex-1" style={{ backgroundColor: colors.bg.primary }}>
        {/* Header */}
        <View
          className="flex-row items-center justify-between px-4 pt-4 pb-3"
          style={{
            backgroundColor: colors.bg.secondary,
            borderBottomWidth: 1,
            borderBottomColor: colors.border.default,
          }}
        >
          <Text className="text-text-primary text-lg font-bold">
            Select Manifest
          </Text>
          <Pressable onPress={onClose} className="active:opacity-70">
            <Text style={{ color: colors.accent.default }} className="text-sm font-semibold">
              Cancel
            </Text>
          </Pressable>
        </View>

        {/* Search */}
        <View className="px-4 py-3">
          <TextInput
            className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm"
            style={{ borderColor: colors.border.default, borderWidth: 1 }}
            placeholder="Search manifests..."
            placeholderTextColor={colors.text.muted}
            value={search}
            onChangeText={setSearch}
            autoFocus
          />
        </View>

        {/* List */}
        {loading ? (
          <View className="py-12 items-center">
            <ActivityIndicator color={colors.accent.default} />
          </View>
        ) : (
          <FlatList
            data={filtered}
            keyExtractor={(m) => m.id}
            contentContainerStyle={{ paddingHorizontal: 16, paddingBottom: 40 }}
            ListEmptyComponent={
              <View className="py-12 items-center">
                <Text className="text-text-muted text-sm">
                  {search ? "No matching manifests" : "No manifests available"}
                </Text>
              </View>
            }
            renderItem={({ item }) => (
              <Pressable
                className="py-3 px-4 mb-2 rounded-card active:opacity-70"
                style={{ backgroundColor: colors.bg.card }}
                onPress={() => {
                  onSelect(item);
                  onClose();
                }}
              >
                <Text
                  className="text-text-primary text-sm font-medium"
                  numberOfLines={1}
                >
                  {item.title}
                </Text>
                <View className="flex-row items-center gap-2 mt-1">
                  <Text
                    className="text-text-muted text-xs"
                    style={{ fontFamily: "Courier" }}
                  >
                    {item.marker}
                  </Text>
                  <View
                    className="px-2 py-0.5 rounded-full"
                    style={{
                      backgroundColor:
                        item.status === "active"
                          ? `${colors.status.success}22`
                          : `${colors.text.muted}22`,
                    }}
                  >
                    <Text
                      className="text-xs"
                      style={{
                        color:
                          item.status === "active"
                            ? colors.status.success
                            : colors.text.muted,
                      }}
                    >
                      {item.status}
                    </Text>
                  </View>
                </View>
                {item.tags.length > 0 && (
                  <View className="flex-row flex-wrap gap-1 mt-1.5">
                    {item.tags.slice(0, 3).map((tag) => (
                      <Text
                        key={tag}
                        className="text-xs"
                        style={{ color: colors.accent.hover }}
                      >
                        #{tag}
                      </Text>
                    ))}
                  </View>
                )}
              </Pressable>
            )}
          />
        )}
      </View>
    </Modal>
  );
}

// ---------------------------------------------------------------------------
// Agent Picker — dropdown of connected agents
// ---------------------------------------------------------------------------

function AgentPicker({
  value,
  onChange,
}: {
  value: string;
  onChange: (agent: string) => void;
}) {
  const { data: agents, isLoading } = useSettingsAgents();
  const connectedAgents = agents?.filter((a) => a.connected) ?? [];

  if (isLoading) {
    return (
      <View className="bg-bg-input rounded-lg px-3 py-2.5" style={{ borderColor: colors.border.default, borderWidth: 1 }}>
        <Text className="text-text-muted text-sm">Loading agents...</Text>
      </View>
    );
  }

  if (connectedAgents.length === 0) {
    return (
      <View>
        <TextInput
          className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm"
          style={{ borderColor: colors.border.default, borderWidth: 1 }}
          placeholder="claude-code (default)"
          placeholderTextColor={colors.text.muted}
          value={value}
          onChangeText={onChange}
          autoCapitalize="none"
          autoCorrect={false}
        />
        <Text className="text-text-muted text-xs mt-1">
          No connected agents found. Type agent name manually.
        </Text>
      </View>
    );
  }

  return (
    <View>
      <View className="flex-row flex-wrap gap-2">
        {connectedAgents.map((agent) => {
          const active = value === agent.id;
          return (
            <Pressable
              key={agent.id}
              onPress={() => onChange(agent.id)}
              className="px-3.5 py-2 rounded-lg active:opacity-70"
              style={{
                backgroundColor: active
                  ? `${colors.status.success}22`
                  : colors.bg.input,
                borderWidth: 1,
                borderColor: active
                  ? colors.status.success
                  : colors.border.default,
              }}
            >
              <Text
                className="text-xs font-medium"
                style={{
                  color: active ? colors.status.success : colors.text.secondary,
                }}
              >
                {agent.name || agent.id}
              </Text>
            </Pressable>
          );
        })}
      </View>
      <TextInput
        className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm mt-2"
        style={{ borderColor: colors.border.default, borderWidth: 1 }}
        placeholder="Or type agent name"
        placeholderTextColor={colors.text.muted}
        value={!connectedAgents.some((a) => a.id === value) ? value : ""}
        onChangeText={onChange}
        autoCapitalize="none"
        autoCorrect={false}
      />
    </View>
  );
}

// ---------------------------------------------------------------------------
// Depends-On field — task dependency
// ---------------------------------------------------------------------------

function DependsOnField({
  value,
  onChange,
}: {
  value: string;
  onChange: (id: string) => void;
}) {
  return (
    <TextInput
      className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm"
      style={{
        borderColor: colors.border.default,
        borderWidth: 1,
        fontFamily: "Courier",
      }}
      placeholder="Task ID or marker (optional)"
      placeholderTextColor={colors.text.muted}
      value={value}
      onChangeText={onChange}
      autoCapitalize="none"
      autoCorrect={false}
    />
  );
}

// ---------------------------------------------------------------------------
// Create Task Screen
// ---------------------------------------------------------------------------

export default function CreateTaskScreen() {
  const createMutation = useCreateTask();
  const { data: manifests, isLoading: loadingManifests } = useManifests();
  const saving = createMutation.isPending;

  // Form state
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [selectedManifest, setSelectedManifest] = useState<Manifest | null>(
    null,
  );
  const [manifestPickerVisible, setManifestPickerVisible] = useState(false);
  const [schedule, setSchedule] = useState("once");
  const [maxTurns, setMaxTurns] = useState(25);
  const [agent, setAgent] = useState("");
  const [dependsOn, setDependsOn] = useState("");

  const handleCreate = () => {
    if (!title.trim()) {
      Alert.alert("Required", "Task name is required.");
      return;
    }
    if (!selectedManifest) {
      Alert.alert("Required", "Select a manifest for this task.");
      return;
    }

    const req: TaskCreateRequest = {
      manifest_id: selectedManifest.id,
      title: title.trim(),
      description: description.trim() || undefined,
      schedule: schedule || "once",
      max_turns: maxTurns,
      agent: agent.trim() || undefined,
      depends_on: dependsOn.trim() || undefined,
    };

    createMutation.mutate(req, {
      onSuccess: () => {
        Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
        router.back();
      },
      onError: (err) => {
        Alert.alert("Error", err.message);
      },
    });
  };

  return (
    <KeyboardAvoidingView
      className="flex-1 bg-bg-primary"
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      keyboardVerticalOffset={100}
    >
      <ScrollView
        className="flex-1 px-4 pt-4"
        contentContainerStyle={{ paddingBottom: 40 }}
        keyboardShouldPersistTaps="handled"
      >
        {/* Manifest Picker */}
        <Label text="Manifest" required />
        <Pressable
          className="bg-bg-input rounded-lg px-3 py-2.5 mb-4 active:opacity-70"
          style={{
            borderColor: selectedManifest
              ? colors.accent.default
              : colors.border.default,
            borderWidth: 1,
          }}
          onPress={() => setManifestPickerVisible(true)}
        >
          {selectedManifest ? (
            <View>
              <Text className="text-text-primary text-sm font-medium" numberOfLines={1}>
                {selectedManifest.title}
              </Text>
              <Text
                className="text-text-muted text-xs mt-0.5"
                style={{ fontFamily: "Courier" }}
              >
                {selectedManifest.marker}
              </Text>
            </View>
          ) : (
            <Text className="text-text-muted text-sm">
              Select a manifest...
            </Text>
          )}
        </Pressable>

        <ManifestPickerModal
          visible={manifestPickerVisible}
          manifests={manifests ?? []}
          loading={loadingManifests}
          onSelect={setSelectedManifest}
          onClose={() => setManifestPickerVisible(false)}
        />

        {/* Task Name */}
        <Label text="Task Name" required />
        <TextInput
          className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm mb-4"
          style={{ borderColor: colors.border.default, borderWidth: 1 }}
          placeholder="What should the agent do?"
          placeholderTextColor={colors.text.muted}
          value={title}
          onChangeText={setTitle}
        />

        {/* Description */}
        <Label text="Description" />
        <TextInput
          className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm mb-4"
          style={{ borderColor: colors.border.default, borderWidth: 1 }}
          placeholder="Additional context for the agent"
          placeholderTextColor={colors.text.muted}
          value={description}
          onChangeText={setDescription}
          multiline
          numberOfLines={3}
          textAlignVertical="top"
        />

        {/* Schedule */}
        <Label text="Schedule" />
        <View className="mb-4">
          <SchedulePicker value={schedule} onChange={setSchedule} />
        </View>

        {/* Max Turns */}
        <Label text="Max Turns" />
        <View className="mb-4">
          <MaxTurnsPicker value={maxTurns} onChange={setMaxTurns} />
        </View>

        {/* Agent */}
        <Label text="Agent" />
        <View className="mb-4">
          <AgentPicker value={agent} onChange={setAgent} />
        </View>

        {/* Depends On */}
        <Label text="Depends On" />
        <View className="mb-6">
          <DependsOnField value={dependsOn} onChange={setDependsOn} />
          <Text className="text-text-muted text-xs mt-1">
            This task will wait until the dependency completes before starting.
          </Text>
        </View>

        {/* Create Button */}
        <Pressable
          className="rounded-card p-4 items-center active:opacity-70 mb-4"
          style={{
            backgroundColor: saving
              ? `${colors.accent.default}88`
              : colors.accent.default,
          }}
          onPress={handleCreate}
          disabled={saving}
        >
          {saving ? (
            <ActivityIndicator color="#fff" size="small" />
          ) : (
            <Text className="text-white font-semibold">Create Task</Text>
          )}
        </Pressable>

        {/* Cancel */}
        <Pressable
          className="rounded-card p-3 items-center active:opacity-70"
          onPress={() => router.back()}
          disabled={saving}
        >
          <Text className="text-text-muted font-medium text-sm">Cancel</Text>
        </Pressable>
      </ScrollView>
    </KeyboardAvoidingView>
  );
}
