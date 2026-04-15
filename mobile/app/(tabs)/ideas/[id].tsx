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
  useIdea,
  useIdeaManifests,
  useDeleteIdea,
  useCreateManifest,
  useLinkIdeaManifest,
} from "@/lib/hooks";
import { StatusBadge } from "@/components/StatusBadge";
import type { IdeaPriority, IdeaStatus, Manifest, ManifestStatus } from "@/lib/types";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const priorityColors: Record<IdeaPriority, string> = {
  low: colors.text.muted,
  medium: colors.status.warning,
  high: "#f97316",
  critical: colors.status.error,
};

const statusMap: Record<
  IdeaStatus,
  "running" | "success" | "warning" | "error" | "idle"
> = {
  new: "idle",
  planned: "warning",
  "in-progress": "running",
  done: "success",
  rejected: "error",
};

const manifestStatusMap: Record<
  ManifestStatus,
  "running" | "success" | "warning" | "error" | "idle"
> = {
  draft: "idle",
  active: "running",
  completed: "success",
  archived: "warning",
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
      <Text className="text-text-secondary text-xs flex-1">{value}</Text>
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
// Linked manifest row
// ---------------------------------------------------------------------------

function ManifestRow({ manifest }: { manifest: Manifest }) {
  const router = useRouter();

  return (
    <Pressable
      className="flex-row items-center py-2.5 px-3 active:opacity-70"
      style={{
        borderBottomWidth: 1,
        borderBottomColor: colors.border.default,
      }}
      onPress={() => router.push(`/manifests/${manifest.id}`)}
    >
      <View className="flex-1">
        <Text className="text-text-primary text-sm" numberOfLines={1}>
          {manifest.title}
        </Text>
        <View className="flex-row items-center gap-2 mt-0.5">
          <Text
            className="text-text-muted text-xs"
            style={{ fontFamily: "Courier" }}
          >
            {manifest.marker}
          </Text>
          <StatusBadge
            status={manifestStatusMap[manifest.status]}
            label={manifest.status}
          />
        </View>
      </View>
      <Text className="text-text-muted text-xs ml-2">{"\u203A"}</Text>
    </Pressable>
  );
}

// ---------------------------------------------------------------------------
// Idea Detail Screen
// ---------------------------------------------------------------------------

export default function IdeaDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();

  const { data: idea, isLoading } = useIdea(id ?? "");
  const { data: manifests } = useIdeaManifests(id ?? "");
  const deleteMutation = useDeleteIdea();
  const createManifestMutation = useCreateManifest();
  const linkMutation = useLinkIdeaManifest();

  const handleDelete = () => {
    Alert.alert(
      "Delete Idea",
      `Delete "${idea?.title}"? It can be restored from Recall.`,
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
    if (!idea) return;

    Alert.alert(
      "Promote to Manifest",
      `Create a new manifest from "${idea.title}"?`,
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
                title: idea.title,
                description: idea.description || undefined,
                status: "draft",
                tags: idea.tags.length > 0 ? idea.tags : undefined,
              },
              {
                onSuccess: (manifest) => {
                  // Link the idea to the new manifest
                  linkMutation.mutate({
                    idea_id: idea.id,
                    manifest_id: manifest.id,
                  });
                  Alert.alert(
                    "Manifest Created",
                    `"${manifest.title}" created as draft and linked.`,
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

  if (!idea) {
    return (
      <View className="flex-1 bg-bg-primary items-center justify-center px-4">
        <Text className="text-text-muted text-sm">Idea not found</Text>
      </View>
    );
  }

  return (
    <ScrollView
      className="flex-1 bg-bg-primary px-4 pt-4"
      contentContainerStyle={{ paddingBottom: 40 }}
    >
      {/* Header */}
      <View className="bg-bg-card rounded-card p-4 mb-4">
        <View className="flex-row items-start justify-between mb-2">
          <Text className="text-text-primary text-lg font-bold flex-1 mr-3">
            {idea.title}
          </Text>
          <StatusBadge
            status={statusMap[idea.status]}
            label={idea.status}
          />
        </View>

        {/* Priority indicator */}
        <View className="flex-row items-center gap-2 mb-3">
          <View
            className="w-2.5 h-2.5 rounded-full"
            style={{
              backgroundColor:
                priorityColors[idea.priority] ?? colors.text.muted,
            }}
          />
          <Text
            className="text-sm font-medium"
            style={{
              color: priorityColors[idea.priority] ?? colors.text.muted,
            }}
          >
            {idea.priority} priority
          </Text>
        </View>

        {idea.description ? (
          <Text className="text-text-secondary text-sm mb-3" selectable>
            {idea.description}
          </Text>
        ) : null}

        <View
          style={{
            borderTopWidth: 1,
            borderTopColor: colors.border.default,
            paddingTop: 8,
          }}
        >
          <MetaRow label="Marker" value={idea.marker} />
          <MetaRow label="Author" value={idea.author} />
          <MetaRow label="Source" value={idea.source_node} />
          <MetaRow label="Created" value={formatDateTime(idea.created_at)} />
          <MetaRow label="Updated" value={formatDateTime(idea.updated_at)} />
        </View>
      </View>

      {/* Tags */}
      {idea.tags.length > 0 && (
        <View className="flex-row flex-wrap gap-2 mb-4">
          {idea.tags.map((tag) => (
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

      {/* Linked Manifests */}
      <Section
        title="Linked Manifests"
        count={manifests?.length}
        defaultExpanded={(manifests?.length ?? 0) > 0}
      >
        {manifests && manifests.length > 0 ? (
          <View className="bg-bg-card rounded-card overflow-hidden">
            {manifests.map((manifest) => (
              <ManifestRow key={manifest.id} manifest={manifest} />
            ))}
          </View>
        ) : (
          <View className="bg-bg-card rounded-card p-4 items-center">
            <Text className="text-text-muted text-xs">
              No linked manifests
            </Text>
          </View>
        )}
      </Section>

      {/* Action Buttons */}
      <View className="flex-row gap-3 mt-2">
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
