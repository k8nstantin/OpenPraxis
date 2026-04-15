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
  useManifest,
  useManifestTasks,
  useManifestIdeas,
  useDeleteManifest,
} from "@/lib/hooks";
import { StatusBadge } from "@/components/StatusBadge";
import type { ManifestStatus, Task, Idea } from "@/lib/types";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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
// Linked task row
// ---------------------------------------------------------------------------

function TaskRow({ task }: { task: Task }) {
  const router = useRouter();
  const statusMap: Record<string, "running" | "success" | "warning" | "error" | "idle"> = {
    pending: "idle",
    scheduled: "warning",
    running: "running",
    completed: "success",
    failed: "error",
    cancelled: "idle",
  };

  return (
    <Pressable
      className="flex-row items-center py-2.5 px-3 active:opacity-70"
      style={{
        borderBottomWidth: 1,
        borderBottomColor: colors.border.default,
      }}
      onPress={() => router.push(`/tasks/${task.id}`)}
    >
      <View className="flex-1">
        <Text
          className="text-text-primary text-sm"
          numberOfLines={1}
        >
          {task.title}
        </Text>
        <View className="flex-row items-center gap-2 mt-0.5">
          <Text className="text-text-muted text-xs" style={{ fontFamily: "Courier" }}>
            {task.marker}
          </Text>
          <StatusBadge
            status={statusMap[task.status] ?? "idle"}
            label={task.status}
          />
        </View>
      </View>
      <Text className="text-text-muted text-xs ml-2">{"\u203A"}</Text>
    </Pressable>
  );
}

// ---------------------------------------------------------------------------
// Linked idea row
// ---------------------------------------------------------------------------

function IdeaRow({ idea }: { idea: Idea }) {
  const router = useRouter();
  const priorityColors: Record<string, string> = {
    low: colors.text.muted,
    medium: colors.status.warning,
    high: colors.status.error,
    critical: colors.status.error,
  };

  return (
    <Pressable
      className="flex-row items-center py-2.5 px-3 active:opacity-70"
      style={{
        borderBottomWidth: 1,
        borderBottomColor: colors.border.default,
      }}
      onPress={() => router.push(`/ideas/${idea.id}`)}
    >
      <View className="flex-1">
        <Text
          className="text-text-primary text-sm"
          numberOfLines={1}
        >
          {idea.title}
        </Text>
        <View className="flex-row items-center gap-2 mt-0.5">
          <Text className="text-text-muted text-xs" style={{ fontFamily: "Courier" }}>
            {idea.marker}
          </Text>
          <View
            className="w-1.5 h-1.5 rounded-full"
            style={{
              backgroundColor: priorityColors[idea.priority] ?? colors.text.muted,
            }}
          />
          <Text className="text-text-muted text-xs">{idea.priority}</Text>
        </View>
      </View>
      <Text className="text-text-muted text-xs ml-2">{"\u203A"}</Text>
    </Pressable>
  );
}

// ---------------------------------------------------------------------------
// Manifest Detail Screen
// ---------------------------------------------------------------------------

export default function ManifestDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();

  const { data: manifest, isLoading } = useManifest(id ?? "");
  const { data: tasks } = useManifestTasks(id ?? "");
  const { data: ideas } = useManifestIdeas(id ?? "");
  const deleteMutation = useDeleteManifest();

  const handleEdit = () => {
    router.push(`/manifests/create?edit=${id}`);
  };

  const handleDelete = () => {
    Alert.alert(
      "Delete Manifest",
      `Delete "${manifest?.title}"? It can be restored from Recall.`,
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

  if (isLoading) {
    return (
      <View className="flex-1 bg-bg-primary items-center justify-center">
        <ActivityIndicator color={colors.accent.default} />
      </View>
    );
  }

  if (!manifest) {
    return (
      <View className="flex-1 bg-bg-primary items-center justify-center px-4">
        <Text className="text-text-muted text-sm">Manifest not found</Text>
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
            {manifest.title}
          </Text>
          <StatusBadge
            status={manifestStatusMap[manifest.status]}
            label={manifest.status}
          />
        </View>

        {manifest.description ? (
          <Text className="text-text-secondary text-sm mb-3">
            {manifest.description}
          </Text>
        ) : null}

        <View
          style={{
            borderTopWidth: 1,
            borderTopColor: colors.border.default,
            paddingTop: 8,
          }}
        >
          <MetaRow label="Marker" value={manifest.marker} />
          <MetaRow label="Author" value={manifest.author} />
          <MetaRow label="Source" value={manifest.source_node} />
          <MetaRow label="Version" value={`v${manifest.version}`} />
          <MetaRow label="Created" value={formatDateTime(manifest.created_at)} />
          <MetaRow label="Updated" value={formatDateTime(manifest.updated_at)} />
        </View>
      </View>

      {/* Tags */}
      {manifest.tags.length > 0 && (
        <View className="flex-row flex-wrap gap-2 mb-4">
          {manifest.tags.map((tag) => (
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

      {/* Jira Refs */}
      {manifest.jira_refs.length > 0 && (
        <View className="flex-row flex-wrap gap-2 mb-4">
          {manifest.jira_refs.map((ref) => (
            <View
              key={ref}
              className="px-3 py-1 rounded-full"
              style={{ backgroundColor: `${colors.status.warning}15` }}
            >
              <Text
                style={{ color: colors.status.warning }}
                className="text-xs font-medium"
              >
                {ref}
              </Text>
            </View>
          ))}
        </View>
      )}

      {/* Content */}
      {manifest.content ? (
        <Section title="Content">
          <View
            className="bg-bg-card rounded-card p-4"
            style={{ borderWidth: 1, borderColor: colors.border.default }}
          >
            <Text
              className="text-text-primary text-xs leading-5"
              style={{
                fontFamily: "Courier",
              }}
              selectable
            >
              {manifest.content}
            </Text>
          </View>
        </Section>
      ) : null}

      {/* Linked Tasks */}
      <Section
        title="Linked Tasks"
        count={tasks?.length}
        defaultExpanded={(tasks?.length ?? 0) > 0}
      >
        {tasks && tasks.length > 0 ? (
          <View className="bg-bg-card rounded-card overflow-hidden">
            {tasks.map((task) => (
              <TaskRow key={task.id} task={task} />
            ))}
          </View>
        ) : (
          <View className="bg-bg-card rounded-card p-4 items-center">
            <Text className="text-text-muted text-xs">No linked tasks</Text>
          </View>
        )}
      </Section>

      {/* Linked Ideas */}
      <Section
        title="Linked Ideas"
        count={ideas?.length}
        defaultExpanded={(ideas?.length ?? 0) > 0}
      >
        {ideas && ideas.length > 0 ? (
          <View className="bg-bg-card rounded-card overflow-hidden">
            {ideas.map((idea) => (
              <IdeaRow key={idea.id} idea={idea} />
            ))}
          </View>
        ) : (
          <View className="bg-bg-card rounded-card p-4 items-center">
            <Text className="text-text-muted text-xs">No linked ideas</Text>
          </View>
        )}
      </Section>

      {/* Action Buttons */}
      <View className="flex-row gap-3 mt-2">
        <Pressable
          className="flex-1 rounded-card p-3.5 items-center active:opacity-70"
          style={{
            backgroundColor: colors.accent.default,
          }}
          onPress={handleEdit}
        >
          <Text className="text-white font-semibold text-sm">Edit</Text>
        </Pressable>
        <Pressable
          className="flex-1 rounded-card p-3.5 items-center active:opacity-70"
          style={{
            backgroundColor: `${colors.status.error}22`,
          }}
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
