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
  useMemory,
  useDeleteMemory,
  useCreateManifest,
} from "@/lib/hooks";
import type { MemoryType } from "@/lib/types";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const typeColors: Record<MemoryType, string> = {
  insight: "#8b5cf6",
  decision: "#3b82f6",
  pattern: "#06b6d4",
  bug: "#e63757",
  context: "#f59e0b",
  reference: "#10b981",
  visceral: "#e63757",
};

function formatDateTime(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleString("en", {
      month: "short",
      day: "numeric",
      year: "numeric",
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
  if (!value) return null;
  return (
    <View className="flex-row py-1.5">
      <Text className="text-text-muted text-xs w-24">{label}</Text>
      <Text className="text-text-secondary text-xs flex-1" selectable>
        {value}
      </Text>
    </View>
  );
}

// ---------------------------------------------------------------------------
// Tier Tab (L0 / L1 / L2)
// ---------------------------------------------------------------------------

type Tier = "l0" | "l1" | "l2";

const TIERS: { key: Tier; label: string; description: string }[] = [
  { key: "l0", label: "L0", description: "One-liner summary" },
  { key: "l1", label: "L1", description: "Paragraph (embedding)" },
  { key: "l2", label: "L2", description: "Full content" },
];

// ---------------------------------------------------------------------------
// Memory Detail Screen
// ---------------------------------------------------------------------------

export default function MemoryDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();

  const { data: memory, isLoading } = useMemory(id ?? "");
  const deleteMutation = useDeleteMemory();
  const createManifestMutation = useCreateManifest();

  const [activeTier, setActiveTier] = useState<Tier>("l0");

  const handleDelete = () => {
    Alert.alert(
      "Delete Memory",
      `Delete this memory? It can be restored from Recall.`,
      [
        { text: "Cancel", style: "cancel" },
        {
          text: "Delete",
          style: "destructive",
          onPress: () => {
            Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Heavy);
            deleteMutation.mutate(id ?? "", {
              onSuccess: () => router.back(),
            });
          },
        },
      ],
    );
  };

  const handlePromoteToManifest = () => {
    if (!memory) return;

    Alert.alert(
      "Promote to Manifest",
      `Create a new manifest from this memory?`,
      [
        { text: "Cancel", style: "cancel" },
        {
          text: "Create",
          onPress: () => {
            Haptics.notificationAsync(
              Haptics.NotificationFeedbackType.Success,
            );
            createManifestMutation.mutate(
              {
                title: memory.l0 || `Memory: ${memory.path}`,
                description: memory.l1 || memory.l0,
                content: memory.l2 || memory.l1 || memory.l0,
                status: "draft",
                tags: memory.tags,
              },
              {
                onSuccess: (manifest) => {
                  Alert.alert(
                    "Manifest Created",
                    `"${manifest.title}" created as draft.`,
                    [
                      {
                        text: "View",
                        onPress: () =>
                          router.push(`/manifests/${manifest.id}`),
                      },
                      { text: "OK" },
                    ],
                  );
                },
                onError: (err) => {
                  Alert.alert("Error", err.message);
                },
              },
            );
          },
        },
      ],
    );
  };

  if (isLoading) {
    return (
      <View className="flex-1 bg-bg-primary items-center justify-center">
        <ActivityIndicator color={colors.accent.default} />
      </View>
    );
  }

  if (!memory) {
    return (
      <View className="flex-1 bg-bg-primary items-center justify-center px-4">
        <Text className="text-text-muted text-sm">Memory not found</Text>
      </View>
    );
  }

  const tierContent = memory[activeTier];
  const tierInfo = TIERS.find((t) => t.key === activeTier)!;

  return (
    <ScrollView
      className="flex-1 bg-bg-primary px-4 pt-4"
      contentContainerStyle={{ paddingBottom: 40 }}
    >
      {/* Header */}
      <View className="bg-bg-card rounded-card p-4 mb-4">
        <View className="flex-row items-start justify-between mb-2">
          <Text className="text-text-primary text-lg font-bold flex-1 mr-3">
            {memory.l0 || memory.path}
          </Text>
          <View
            className="px-2.5 py-1 rounded-full"
            style={{
              backgroundColor: `${typeColors[memory.type] ?? colors.text.muted}20`,
            }}
          >
            <Text
              className="text-xs font-semibold"
              style={{
                color: typeColors[memory.type] ?? colors.text.muted,
              }}
            >
              {memory.type}
            </Text>
          </View>
        </View>

        <View
          style={{
            borderTopWidth: 1,
            borderTopColor: colors.border.default,
            paddingTop: 8,
          }}
        >
          <MetaRow label="Path" value={memory.path} />
          <MetaRow label="Scope" value={memory.scope} />
          <MetaRow label="Project" value={memory.project} />
          <MetaRow label="Domain" value={memory.domain} />
          <MetaRow label="Source" value={memory.source_node} />
          <MetaRow label="Agent" value={memory.source_agent} />
          <MetaRow label="Accessed" value={`${memory.access_count}x`} />
          <MetaRow label="Created" value={formatDateTime(memory.created_at)} />
          <MetaRow label="Updated" value={formatDateTime(memory.updated_at)} />
          <MetaRow
            label="Last Access"
            value={formatDateTime(memory.accessed_at)}
          />
        </View>
      </View>

      {/* Tags */}
      {memory.tags.length > 0 && (
        <View className="flex-row flex-wrap gap-2 mb-4">
          {memory.tags.map((tag) => (
            <View
              key={tag}
              className="px-3 py-1 rounded-full"
              style={{ backgroundColor: `${colors.accent.default}15` }}
            >
              <Text
                style={{ color: colors.accent.hover }}
                className="text-xs"
              >
                {tag}
              </Text>
            </View>
          ))}
        </View>
      )}

      {/* L0/L1/L2 Tier Tabs */}
      <View className="mb-2">
        <Text className="text-text-secondary text-sm font-semibold uppercase tracking-wider mb-2">
          Content Tiers
        </Text>
        <View className="flex-row gap-2 mb-3">
          {TIERS.map((tier) => {
            const active = activeTier === tier.key;
            const hasContent = !!memory[tier.key];
            return (
              <Pressable
                key={tier.key}
                onPress={() => setActiveTier(tier.key)}
                className="flex-1 rounded-lg py-2.5 items-center active:opacity-70"
                style={{
                  backgroundColor: active
                    ? colors.accent.default
                    : `${colors.accent.default}15`,
                  opacity: hasContent ? 1 : 0.4,
                }}
              >
                <Text
                  className="text-sm font-bold"
                  style={{ color: active ? "#fff" : colors.accent.hover }}
                >
                  {tier.label}
                </Text>
                <Text
                  className="text-xs mt-0.5"
                  style={{
                    color: active
                      ? "rgba(255,255,255,0.7)"
                      : colors.text.muted,
                  }}
                >
                  {tier.description}
                </Text>
              </Pressable>
            );
          })}
        </View>

        {/* Tier Content */}
        <View
          className="bg-bg-card rounded-card p-4"
          style={{ borderWidth: 1, borderColor: colors.border.default }}
        >
          {tierContent ? (
            <Text
              className="text-text-primary text-sm leading-6"
              selectable
            >
              {tierContent}
            </Text>
          ) : (
            <Text className="text-text-muted text-sm text-center py-4">
              No {tierInfo.label} content available
            </Text>
          )}
        </View>
      </View>

      {/* Action Buttons */}
      <View className="flex-row gap-3 mt-4">
        <Pressable
          className="flex-1 rounded-card p-3.5 items-center active:opacity-70"
          style={{ backgroundColor: colors.accent.default }}
          onPress={handlePromoteToManifest}
          disabled={createManifestMutation.isPending}
        >
          <Text className="text-white font-semibold text-sm">
            {createManifestMutation.isPending
              ? "Creating..."
              : "Promote to Manifest"}
          </Text>
        </Pressable>
        <Pressable
          className="flex-1 rounded-card p-3.5 items-center active:opacity-70"
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
      </View>
    </ScrollView>
  );
}
