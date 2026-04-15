import { useCallback, useRef, useState } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  RefreshControl,
  TextInput,
  Alert,
} from "react-native";
import { useQueryClient } from "@tanstack/react-query";
import Swipeable from "react-native-gesture-handler/Swipeable";
import * as Haptics from "expo-haptics";
import { colors, tabIcons } from "@/lib/theme";
import { useAppStore } from "@/lib/store";
import {
  useVisceralByPeer,
  useAddVisceralRule,
  useDeleteVisceralRule,
  useVisceralConfirmations,
  queryKeys,
} from "@/lib/hooks";
import { PeerTree } from "@/components/PeerTree";
import type { Memory, VisceralConfirmation } from "@/lib/types";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatDate(iso: string): string {
  try {
    const d = new Date(iso);
    const month = d.toLocaleString("en", { month: "short" });
    return `${month} ${d.getDate()}`;
  } catch {
    return "";
  }
}

function formatDateTime(iso: string): string {
  try {
    const d = new Date(iso);
    const month = d.toLocaleString("en", { month: "short" });
    const time = d.toLocaleTimeString("en", {
      hour: "2-digit",
      minute: "2-digit",
    });
    return `${month} ${d.getDate()}, ${time}`;
  } catch {
    return "";
  }
}

// ---------------------------------------------------------------------------
// Rule Row (with swipe-to-delete)
// ---------------------------------------------------------------------------

function RuleRow({
  rule,
  onDelete,
}: {
  rule: Memory;
  onDelete: (id: string) => void;
}) {
  const swipeableRef = useRef<Swipeable>(null);

  const renderRightActions = () => (
    <Pressable
      className="justify-center px-6"
      style={{ backgroundColor: colors.status.error }}
      onPress={() => {
        swipeableRef.current?.close();
        onDelete(rule.id);
      }}
    >
      <Text className="text-white font-semibold text-sm">Delete</Text>
    </Pressable>
  );

  return (
    <Swipeable
      ref={swipeableRef}
      renderRightActions={renderRightActions}
      rightThreshold={40}
      overshootRight={false}
    >
      <View
        className="py-3 px-4"
        style={{ backgroundColor: colors.bg.card }}
      >
        {/* Rule text */}
        <Text className="text-text-primary text-sm font-medium">
          {rule.l0}
        </Text>

        {/* Meta line: marker + date */}
        <View className="flex-row items-center mt-1.5 gap-2">
          <Text
            className="text-text-muted text-xs"
            style={{ fontFamily: "Courier" }}
          >
            {rule.path.replace(/^\/visceral\//, "")}
          </Text>
          <Text className="text-text-muted text-xs">
            {formatDate(rule.created_at)}
          </Text>
        </View>
      </View>
    </Swipeable>
  );
}

// ---------------------------------------------------------------------------
// Confirmation Row
// ---------------------------------------------------------------------------

function ConfirmationRow({
  confirmation,
}: {
  confirmation: VisceralConfirmation;
}) {
  return (
    <View
      className="flex-row items-center py-2.5 px-4"
      style={{
        backgroundColor: colors.bg.card,
        borderBottomWidth: 1,
        borderBottomColor: colors.border.default,
      }}
    >
      <View
        className="w-2 h-2 rounded-full mr-3"
        style={{ backgroundColor: colors.status.success }}
      />
      <View className="flex-1">
        <Text className="text-text-secondary text-xs">
          Session confirmed {confirmation.rules_count} rule
          {confirmation.rules_count === 1 ? "" : "s"}
        </Text>
      </View>
      <Text className="text-text-muted text-xs">
        {formatDateTime(confirmation.created_at)}
      </Text>
    </View>
  );
}

// ---------------------------------------------------------------------------
// Visceral Screen
// ---------------------------------------------------------------------------

export default function VisceralScreen() {
  const queryClient = useQueryClient();
  const connected = useAppStore((s) => s.connected);

  const { data: byPeer, isLoading } = useVisceralByPeer();
  const { data: confirmations } = useVisceralConfirmations(10);
  const addMutation = useAddVisceralRule();
  const deleteMutation = useDeleteVisceralRule();

  const [refreshing, setRefreshing] = useState(false);
  const [newRule, setNewRule] = useState("");
  const [showConfirmations, setShowConfirmations] = useState(false);
  const inputRef = useRef<TextInput>(null);

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    try {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.visceralByPeer }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.visceralConfirmations,
        }),
      ]);
    } finally {
      setRefreshing(false);
    }
  }, [queryClient]);

  const handleAdd = useCallback(() => {
    const text = newRule.trim();
    if (!text) return;

    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    addMutation.mutate(text, {
      onSuccess: () => {
        setNewRule("");
        inputRef.current?.blur();
      },
    });
  }, [newRule, addMutation]);

  const handleDelete = useCallback(
    (id: string) => {
      const rule = Object.values(byPeer ?? {})
        .flat()
        .find((r) => r.id === id);
      const text = rule?.l0 ?? "this rule";

      Alert.alert(
        "Delete Rule",
        `Delete "${text}"? This cannot be undone.`,
        [
          { text: "Cancel", style: "cancel" },
          {
            text: "Delete",
            style: "destructive",
            onPress: () => {
              Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
              deleteMutation.mutate(id);
            },
          },
        ],
      );
    },
    [byPeer, deleteMutation],
  );

  // Totals
  const totalCount = Object.values(byPeer ?? {}).reduce(
    (sum, arr) => sum + arr.length,
    0,
  );

  return (
    <View className="flex-1 bg-bg-primary">
      <ScrollView
        className="flex-1"
        contentContainerStyle={{ paddingBottom: 40 }}
        keyboardShouldPersistTaps="handled"
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            onRefresh={onRefresh}
            tintColor={colors.text.muted}
          />
        }
      >
        {/* Header */}
        <View className="px-4 pt-4 pb-2">
          <Text className="text-text-muted text-xs">
            Mandatory operating rules that override all other behavior.
            Non-negotiable constraints set by the user.
          </Text>
        </View>

        {/* Add Rule Input */}
        {connected && (
          <View className="px-4 pb-4">
            <View
              className="flex-row items-center rounded-lg px-3"
              style={{
                backgroundColor: colors.bg.input,
                borderColor: colors.border.default,
                borderWidth: 1,
              }}
            >
              <Text
                className="text-sm mr-2"
                style={{ color: colors.status.error }}
              >
                {tabIcons.visceral}
              </Text>
              <TextInput
                ref={inputRef}
                className="flex-1 py-2.5 text-text-primary text-sm"
                placeholder="Add a new rule..."
                placeholderTextColor={colors.text.muted}
                value={newRule}
                onChangeText={setNewRule}
                onSubmitEditing={handleAdd}
                returnKeyType="done"
                editable={!addMutation.isPending}
              />
              {newRule.length > 0 && (
                <Pressable
                  onPress={handleAdd}
                  className="px-3 py-1.5 rounded-md active:opacity-70"
                  style={{ backgroundColor: `${colors.status.error}33` }}
                  disabled={addMutation.isPending}
                >
                  <Text
                    className="text-xs font-semibold"
                    style={{ color: colors.status.error }}
                  >
                    {addMutation.isPending ? "Adding..." : "Add"}
                  </Text>
                </Pressable>
              )}
            </View>
          </View>
        )}

        {/* Count header */}
        {connected && byPeer && (
          <View className="px-4 pb-3 flex-row items-center justify-between">
            <Text className="text-text-secondary text-sm font-semibold uppercase tracking-wider">
              Rules
            </Text>
            <Text className="text-text-muted text-xs">
              {totalCount} total
            </Text>
          </View>
        )}

        {/* Rules by Peer */}
        <View className="px-4">
          {!connected ? (
            <View className="bg-bg-card rounded-card p-6 items-center">
              <Text className="text-3xl mb-3">{tabIcons.visceral}</Text>
              <Text className="text-text-muted text-sm text-center">
                Connect to a peer to view visceral rules
              </Text>
            </View>
          ) : (
            <PeerTree
              data={byPeer}
              renderItem={(rule) => (
                <RuleRow rule={rule} onDelete={handleDelete} />
              )}
              keyExtractor={(r) => r.id}
              emptyIcon={tabIcons.visceral}
              emptyMessage="No visceral rules yet"
              loading={isLoading}
            />
          )}
        </View>

        {/* Confirmations Section */}
        {connected && confirmations && confirmations.length > 0 && (
          <View className="px-4 mt-6">
            <Pressable
              className="flex-row items-center justify-between mb-3"
              onPress={() => setShowConfirmations(!showConfirmations)}
            >
              <Text className="text-text-secondary text-sm font-semibold uppercase tracking-wider">
                Recent Confirmations
              </Text>
              <View className="flex-row items-center gap-2">
                <View
                  className="px-2 py-0.5 rounded-full"
                  style={{ backgroundColor: `${colors.status.success}22` }}
                >
                  <Text
                    style={{ color: colors.status.success }}
                    className="text-xs font-bold"
                  >
                    {confirmations.length}
                  </Text>
                </View>
                <Text className="text-text-muted text-xs">
                  {showConfirmations ? "\u25BC" : "\u25B6"}
                </Text>
              </View>
            </Pressable>

            {showConfirmations && (
              <View className="bg-bg-card rounded-card overflow-hidden">
                {confirmations.map((c) => (
                  <ConfirmationRow key={c.id} confirmation={c} />
                ))}
              </View>
            )}
          </View>
        )}
      </ScrollView>
    </View>
  );
}
