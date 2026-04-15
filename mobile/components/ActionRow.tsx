import { useState } from "react";
import { View, Text, Pressable } from "react-native";
import { colors } from "@/lib/theme";
import type { Action } from "@/lib/types";

// ---------------------------------------------------------------------------
// Tool name badge colors (same palette as TaskOutput)
// ---------------------------------------------------------------------------

const toolColors: Record<string, string> = {
  Bash: "#f59e0b",
  Read: "#3b82f6",
  Write: "#8b5cf6",
  Edit: "#10b981",
  Grep: "#06b6d4",
  Glob: "#6366f1",
  Agent: "#ec4899",
  TodoWrite: "#14b8a6",
  WebSearch: "#f97316",
  WebFetch: "#f97316",
};

function getToolColor(name: string): string {
  return toolColors[name] || colors.accent.default;
}

function formatDateTime(iso: string): string {
  if (!iso) return "\u2014";
  try {
    const d = new Date(iso);
    return d.toLocaleString("en", {
      hour: "numeric",
      minute: "2-digit",
      second: "2-digit",
    });
  } catch {
    return iso;
  }
}

function extractPreview(input: string): string {
  if (!input) return "";
  try {
    const parsed = JSON.parse(input);
    if (parsed.command) return parsed.command;
    if (parsed.file_path) return parsed.file_path;
    if (parsed.pattern) return parsed.pattern;
    if (parsed.query) return parsed.query;
    if (parsed.content) return parsed.content.substring(0, 80);
    return input.substring(0, 80);
  } catch {
    return input.substring(0, 80);
  }
}

// ---------------------------------------------------------------------------
// ActionRow
// ---------------------------------------------------------------------------

export function ActionRow({ action }: { action: Action }) {
  const [expanded, setExpanded] = useState(false);
  const badgeColor = getToolColor(action.tool_name);
  const preview = extractPreview(action.tool_input);

  return (
    <Pressable
      onPress={() => setExpanded(!expanded)}
      style={{
        borderBottomWidth: 1,
        borderBottomColor: colors.border.default,
      }}
    >
      <View className="py-2 px-3">
        <View className="flex-row items-center gap-2">
          <View
            className="px-1.5 py-0.5 rounded"
            style={{ backgroundColor: `${badgeColor}22` }}
          >
            <Text
              style={{ color: badgeColor, fontSize: 10, fontWeight: "600" }}
            >
              {action.tool_name}
            </Text>
          </View>
          <Text
            className="flex-1 text-xs"
            style={{
              color: colors.text.secondary,
              fontFamily: "Menlo",
              fontSize: 11,
            }}
            numberOfLines={expanded ? undefined : 1}
            selectable={expanded}
          >
            {preview}
          </Text>
          <Text className="text-text-muted" style={{ fontSize: 10 }}>
            {formatDateTime(action.created_at)}
          </Text>
        </View>

        {expanded && action.tool_response && (
          <View
            className="mt-2 p-2 rounded"
            style={{ backgroundColor: colors.bg.input }}
          >
            <Text
              className="text-xs"
              style={{
                color: colors.text.muted,
                fontFamily: "Menlo",
                fontSize: 10,
              }}
              selectable
            >
              {action.tool_response.length > 500
                ? action.tool_response.substring(0, 500) + "..."
                : action.tool_response}
            </Text>
          </View>
        )}
      </View>
    </Pressable>
  );
}
