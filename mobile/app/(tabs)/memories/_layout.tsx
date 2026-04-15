import { Stack } from "expo-router";
import { colors } from "@/lib/theme";

export default function MemoriesLayout() {
  return (
    <Stack
      screenOptions={{
        headerStyle: { backgroundColor: colors.bg.secondary },
        headerTintColor: colors.text.primary,
        headerTitleStyle: { fontWeight: "600" },
        contentStyle: { backgroundColor: colors.bg.primary },
      }}
    >
      <Stack.Screen name="index" options={{ title: "Memories" }} />
      <Stack.Screen name="[id]" options={{ title: "Memory" }} />
    </Stack>
  );
}
