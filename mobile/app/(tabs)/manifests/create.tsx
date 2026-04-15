import { useState, useEffect } from "react";
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
} from "react-native";
import { router, useLocalSearchParams, useNavigation } from "expo-router";
import * as Haptics from "expo-haptics";
import { colors } from "@/lib/theme";
import {
  useManifest,
  useCreateManifest,
  useUpdateManifest,
} from "@/lib/hooks";
import type { ManifestStatus } from "@/lib/types";

// ---------------------------------------------------------------------------
// Status picker
// ---------------------------------------------------------------------------

const STATUSES: { key: ManifestStatus; label: string; color: string }[] = [
  { key: "draft", label: "Draft", color: colors.text.muted },
  { key: "active", label: "Active", color: colors.status.success },
  { key: "completed", label: "Done", color: colors.accent.default },
  { key: "archived", label: "Archived", color: colors.status.warning },
];

function StatusPicker({
  value,
  onChange,
}: {
  value: ManifestStatus;
  onChange: (s: ManifestStatus) => void;
}) {
  return (
    <View className="flex-row gap-2">
      {STATUSES.map((s) => {
        const active = value === s.key;
        return (
          <Pressable
            key={s.key}
            onPress={() => onChange(s.key)}
            className="flex-1 py-2 rounded-lg items-center active:opacity-70"
            style={{
              backgroundColor: active ? `${s.color}22` : colors.bg.input,
              borderWidth: 1,
              borderColor: active ? s.color : colors.border.default,
            }}
          >
            <Text
              className="text-xs font-semibold"
              style={{ color: active ? s.color : colors.text.muted }}
            >
              {s.label}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

// ---------------------------------------------------------------------------
// Form label
// ---------------------------------------------------------------------------

function Label({ text, required }: { text: string; required?: boolean }) {
  return (
    <Text className="text-text-muted text-xs mb-2">
      {text}
      {required && (
        <Text style={{ color: colors.status.error }}> *</Text>
      )}
    </Text>
  );
}

// ---------------------------------------------------------------------------
// Create / Edit Manifest Screen
// ---------------------------------------------------------------------------

export default function CreateManifestScreen() {
  const params = useLocalSearchParams<{ edit?: string }>();
  const navigation = useNavigation();
  const editId = params.edit;
  const isEdit = !!editId;

  // Load existing manifest when editing
  const { data: existing, isLoading: loadingExisting } = useManifest(
    editId ?? "",
  );

  const createMutation = useCreateManifest();
  const updateMutation = useUpdateManifest();
  const saving = createMutation.isPending || updateMutation.isPending;

  // Form state
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [content, setContent] = useState("");
  const [status, setStatus] = useState<ManifestStatus>("draft");
  const [tagsText, setTagsText] = useState("");
  const [jiraRefsText, setJiraRefsText] = useState("");

  // Set header title for edit mode
  useEffect(() => {
    if (isEdit) {
      navigation.setOptions({ title: "Edit Manifest" });
    }
  }, [isEdit, navigation]);

  // Pre-fill form when editing
  useEffect(() => {
    if (existing && isEdit) {
      setTitle(existing.title);
      setDescription(existing.description ?? "");
      setContent(existing.content ?? "");
      setStatus(existing.status);
      setTagsText(existing.tags?.join(", ") ?? "");
      setJiraRefsText(existing.jira_refs?.join(", ") ?? "");
    }
  }, [existing, isEdit]);

  const parseTags = (text: string): string[] =>
    text
      .split(",")
      .map((t) => t.trim())
      .filter(Boolean);

  const handleSave = () => {
    if (!title.trim()) {
      Alert.alert("Required", "Title is required.");
      return;
    }

    const tags = parseTags(tagsText);
    const jira_refs = parseTags(jiraRefsText);

    if (isEdit && editId) {
      updateMutation.mutate(
        {
          id: editId,
          req: {
            title: title.trim(),
            description: description.trim() || undefined,
            content: content.trim() || undefined,
            status,
            tags: tags.length > 0 ? tags : undefined,
            jira_refs: jira_refs.length > 0 ? jira_refs : undefined,
          },
        },
        {
          onSuccess: () => {
            Haptics.notificationAsync(
              Haptics.NotificationFeedbackType.Success,
            );
            router.back();
          },
          onError: (err) => {
            Alert.alert("Error", err.message);
          },
        },
      );
    } else {
      createMutation.mutate(
        {
          title: title.trim(),
          description: description.trim() || undefined,
          content: content.trim() || undefined,
          status,
          tags: tags.length > 0 ? tags : undefined,
          jira_refs: jira_refs.length > 0 ? jira_refs : undefined,
        },
        {
          onSuccess: () => {
            Haptics.notificationAsync(
              Haptics.NotificationFeedbackType.Success,
            );
            router.back();
          },
          onError: (err) => {
            Alert.alert("Error", err.message);
          },
        },
      );
    }
  };

  if (isEdit && loadingExisting) {
    return (
      <View className="flex-1 bg-bg-primary items-center justify-center">
        <ActivityIndicator color={colors.accent.default} />
      </View>
    );
  }

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
        {/* Title */}
        <Label text="Title" required />
        <TextInput
          className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm mb-4"
          style={{ borderColor: colors.border.default, borderWidth: 1 }}
          placeholder="Manifest title"
          placeholderTextColor={colors.text.muted}
          value={title}
          onChangeText={setTitle}
          autoFocus={!isEdit}
        />

        {/* Description */}
        <Label text="Description" />
        <TextInput
          className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm mb-4"
          style={{ borderColor: colors.border.default, borderWidth: 1 }}
          placeholder="Brief description"
          placeholderTextColor={colors.text.muted}
          value={description}
          onChangeText={setDescription}
          multiline
          numberOfLines={2}
        />

        {/* Content (Markdown) */}
        <Label text="Content (Markdown)" />
        <TextInput
          className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm mb-4"
          style={{
            borderColor: colors.border.default,
            borderWidth: 1,
            minHeight: 200,
            fontFamily: Platform.OS === "ios" ? "Menlo" : "monospace",
            fontSize: 13,
            lineHeight: 20,
          }}
          placeholder="Write your manifest spec here..."
          placeholderTextColor={colors.text.muted}
          value={content}
          onChangeText={setContent}
          multiline
          textAlignVertical="top"
        />

        {/* Status */}
        <Label text="Status" />
        <View className="mb-4">
          <StatusPicker value={status} onChange={setStatus} />
        </View>

        {/* Tags */}
        <Label text="Tags (comma-separated)" />
        <TextInput
          className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm mb-4"
          style={{ borderColor: colors.border.default, borderWidth: 1 }}
          placeholder="mobile, api, infrastructure"
          placeholderTextColor={colors.text.muted}
          value={tagsText}
          onChangeText={setTagsText}
          autoCapitalize="none"
        />

        {/* Jira Refs */}
        <Label text="Jira Tickets (comma-separated)" />
        <TextInput
          className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm mb-4"
          style={{ borderColor: colors.border.default, borderWidth: 1 }}
          placeholder="ENG-1234, ENG-5678"
          placeholderTextColor={colors.text.muted}
          value={jiraRefsText}
          onChangeText={setJiraRefsText}
          autoCapitalize="characters"
        />

        {/* Save Button */}
        <Pressable
          className="rounded-card p-4 items-center active:opacity-70 mb-4"
          style={{
            backgroundColor: saving
              ? `${colors.accent.default}88`
              : colors.accent.default,
          }}
          onPress={handleSave}
          disabled={saving}
        >
          {saving ? (
            <ActivityIndicator color="#fff" size="small" />
          ) : (
            <Text className="text-white font-semibold">
              {isEdit ? "Update Manifest" : "Create Manifest"}
            </Text>
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
