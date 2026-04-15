import { Stack } from "expo-router";
import { colors } from "@/lib/theme";

export default function IdeasLayout() {
  return (
    <Stack
      screenOptions={{
        headerStyle: { backgroundColor: colors.bg.secondary },
        headerTintColor: colors.text.primary,
        headerTitleStyle: { fontWeight: "600" },
        contentStyle: { backgroundColor: colors.bg.primary },
      }}
    >
      <Stack.Screen name="index" options={{ title: "Ideas" }} />
      <Stack.Screen name="[id]" options={{ title: "Idea" }} />
      <Stack.Screen name="create" options={{ title: "New Idea", presentation: "modal" }} />
    </Stack>
  );
}
