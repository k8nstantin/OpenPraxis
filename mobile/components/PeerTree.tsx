import { useState } from "react";
import { View, Text, Pressable, ActivityIndicator } from "react-native";
import { colors } from "@/lib/theme";

interface PeerTreeProps<T> {
  data: Record<string, T[]> | undefined;
  renderItem: (item: T) => React.ReactNode;
  keyExtractor: (item: T) => string;
  emptyIcon?: string;
  emptyMessage?: string;
  loading?: boolean;
}

function PeerSection<T>({
  peer,
  items,
  renderItem,
  keyExtractor,
}: {
  peer: string;
  items: T[];
  renderItem: (item: T) => React.ReactNode;
  keyExtractor: (item: T) => string;
}) {
  const [expanded, setExpanded] = useState(true);

  return (
    <View className="mb-3">
      <Pressable
        onPress={() => setExpanded(!expanded)}
        className="flex-row items-center py-3 px-4 active:opacity-70"
        style={{
          backgroundColor: colors.bg.card,
          borderTopLeftRadius: 12,
          borderTopRightRadius: 12,
          borderBottomLeftRadius: expanded ? 0 : 12,
          borderBottomRightRadius: expanded ? 0 : 12,
        }}
      >
        <Text className="text-text-muted text-xs mr-2">
          {expanded ? "\u25BC" : "\u25B6"}
        </Text>
        <Text
          className="text-text-secondary text-sm font-medium flex-1"
          numberOfLines={1}
        >
          {peer}
        </Text>
        <View
          className="px-2 py-0.5 rounded-full"
          style={{ backgroundColor: `${colors.accent.default}22` }}
        >
          <Text
            style={{ color: colors.accent.default }}
            className="text-xs font-bold"
          >
            {items.length}
          </Text>
        </View>
      </Pressable>
      {expanded && (
        <View
          style={{
            backgroundColor: colors.bg.card,
            borderTopWidth: 1,
            borderTopColor: colors.border.default,
            borderBottomLeftRadius: 12,
            borderBottomRightRadius: 12,
          }}
        >
          {items.map((item, index) => (
            <View
              key={keyExtractor(item)}
              style={
                index < items.length - 1
                  ? {
                      borderBottomWidth: 1,
                      borderBottomColor: colors.border.default,
                    }
                  : undefined
              }
            >
              {renderItem(item)}
            </View>
          ))}
        </View>
      )}
    </View>
  );
}

export function PeerTree<T>({
  data,
  renderItem,
  keyExtractor,
  emptyIcon,
  emptyMessage,
  loading,
}: PeerTreeProps<T>) {
  if (loading) {
    return (
      <View className="py-12 items-center">
        <ActivityIndicator color={colors.accent.default} />
      </View>
    );
  }

  if (!data || Object.keys(data).length === 0) {
    return (
      <View className="bg-bg-card rounded-card p-6 items-center">
        {emptyIcon && <Text className="text-3xl mb-3">{emptyIcon}</Text>}
        <Text className="text-text-muted text-sm">
          {emptyMessage ?? "No items"}
        </Text>
      </View>
    );
  }

  return (
    <View>
      {Object.entries(data).map(([peer, items]) => (
        <PeerSection
          key={peer}
          peer={peer}
          items={items}
          renderItem={renderItem}
          keyExtractor={keyExtractor}
        />
      ))}
    </View>
  );
}
