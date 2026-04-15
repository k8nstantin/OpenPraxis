import { useState } from "react";
import {
  View,
  Text,
  TextInput,
  Pressable,
  ScrollView,
  Alert,
  KeyboardAvoidingView,
  Platform,
} from "react-native";
import { router } from "expo-router";
import * as Haptics from "expo-haptics";
import { colors } from "@/lib/theme";
import { useCreateIdea } from "@/lib/hooks";
import type { IdeaPriority } from "@/lib/types";

// ---------------------------------------------------------------------------
// Priority config
// ---------------------------------------------------------------------------

const PRIORITIES: { key: IdeaPriority; label: string; color: string }[] = [
  { key: "low", label: "Low", color: colors.text.muted },
  { key: "medium", label: "Medium", color: colors.status.warning },
  { key: "high", label: "High", color: "#f97316" },
  { key: "critical", label: "Critical", color: colors.status.error },
];

// ---------------------------------------------------------------------------
// Create Idea Screen
// ---------------------------------------------------------------------------

export default function CreateIdeaScreen() {
  const createMutation = useCreateIdea();

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState<IdeaPriority>("medium");
  const [tags, setTags] = useState("");

  const canSave = title.trim().length > 0;

  const handleSave = () => {
    if (!canSave) return;

    const parsedTags = tags
      .split(",")
      .map((t) => t.trim())
      .filter(Boolean);

    Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    createMutation.mutate(
      {
        title: title.trim(),
        description: description.trim() || undefined,
        priority,
        tags: parsedTags.length > 0 ? parsedTags : undefined,
      },
      {
        onSuccess: () => {
          router.back();
        },
        onError: (err) => {
          Alert.alert("Error", err.message);
        },
      },
    );
  };

  return (
    <KeyboardAvoidingView
      className="flex-1 bg-bg-primary"
      behavior={Platform.OS === "ios" ? "padding" : undefined}
    >
      <ScrollView
        className="flex-1 px-4 pt-4"
        contentContainerStyle={{ paddingBottom: 40 }}
        keyboardShouldPersistTaps="handled"
      >
        {/* Title */}
        <Text className="text-text-muted text-xs mb-2">Title *</Text>
        <TextInput
          className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm mb-4"
          style={{ borderColor: colors.border.default, borderWidth: 1 }}
          placeholder="What's the idea?"
          placeholderTextColor={colors.text.muted}
          value={title}
          onChangeText={setTitle}
          autoFocus
          returnKeyType="next"
        />

        {/* Description (optional) */}
        <Text className="text-text-muted text-xs mb-2">
          Description (optional)
        </Text>
        <TextInput
          className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm mb-4"
          style={{
            borderColor: colors.border.default,
            borderWidth: 1,
            minHeight: 80,
            textAlignVertical: "top",
          }}
          placeholder="More detail..."
          placeholderTextColor={colors.text.muted}
          value={description}
          onChangeText={setDescription}
          multiline
          numberOfLines={3}
        />

        {/* Priority */}
        <Text className="text-text-muted text-xs mb-2">Priority</Text>
        <View className="flex-row gap-2 mb-4">
          {PRIORITIES.map((p) => {
            const active = priority === p.key;
            return (
              <Pressable
                key={p.key}
                onPress={() => setPriority(p.key)}
                className="flex-1 rounded-lg py-3 items-center active:opacity-70"
                style={{
                  backgroundColor: active ? `${p.color}25` : colors.bg.input,
                  borderColor: active ? p.color : colors.border.default,
                  borderWidth: 1,
                }}
              >
                <View
                  className="w-2 h-2 rounded-full mb-1"
                  style={{ backgroundColor: p.color }}
                />
                <Text
                  className="text-xs font-medium"
                  style={{ color: active ? p.color : colors.text.secondary }}
                >
                  {p.label}
                </Text>
              </Pressable>
            );
          })}
        </View>

        {/* Tags */}
        <Text className="text-text-muted text-xs mb-2">
          Tags (comma-separated, optional)
        </Text>
        <TextInput
          className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm mb-6"
          style={{ borderColor: colors.border.default, borderWidth: 1 }}
          placeholder="mobile, ux, backend"
          placeholderTextColor={colors.text.muted}
          value={tags}
          onChangeText={setTags}
        />

        {/* Save button */}
        <Pressable
          className="rounded-card p-4 items-center active:opacity-70 mb-3"
          style={{
            backgroundColor: canSave
              ? colors.accent.default
              : `${colors.accent.default}40`,
          }}
          onPress={handleSave}
          disabled={!canSave || createMutation.isPending}
        >
          <Text className="text-white font-semibold">
            {createMutation.isPending ? "Saving..." : "Save Idea"}
          </Text>
        </Pressable>

        {/* Cancel */}
        <Pressable
          className="rounded-card p-3 items-center active:opacity-70"
          onPress={() => router.back()}
        >
          <Text className="text-text-muted text-sm">Cancel</Text>
        </Pressable>
      </ScrollView>
    </KeyboardAvoidingView>
  );
}
