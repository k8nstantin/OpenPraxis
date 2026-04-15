import { Tabs } from "expo-router";
import { Text, StyleSheet } from "react-native";
import { colors, tabIcons } from "@/lib/theme";

function TabIcon({ icon, focused }: { icon: string; focused: boolean }) {
  return (
    <Text
      style={[
        styles.icon,
        { color: focused ? colors.accent.default : colors.text.muted },
      ]}
    >
      {icon}
    </Text>
  );
}

export default function TabLayout() {
  return (
    <Tabs
      screenOptions={{
        headerStyle: {
          backgroundColor: colors.bg.secondary,
        },
        headerTintColor: colors.text.primary,
        headerTitleStyle: {
          fontWeight: "600",
        },
        tabBarStyle: {
          backgroundColor: colors.bg.sidebar,
          borderTopColor: colors.border.default,
          borderTopWidth: StyleSheet.hairlineWidth,
          paddingTop: 4,
        },
        tabBarActiveTintColor: colors.accent.default,
        tabBarInactiveTintColor: colors.text.muted,
        tabBarLabelStyle: {
          fontSize: 10,
          fontWeight: "500",
        },
      }}
    >
      <Tabs.Screen
        name="index"
        options={{
          title: "Overview",
          tabBarIcon: ({ focused }) => (
            <TabIcon icon={tabIcons.overview} focused={focused} />
          ),
        }}
      />
      <Tabs.Screen
        name="manifests"
        options={{
          title: "Manifests",
          headerShown: false,
          tabBarIcon: ({ focused }) => (
            <TabIcon icon={tabIcons.manifests} focused={focused} />
          ),
        }}
      />
      <Tabs.Screen
        name="tasks"
        options={{
          title: "Tasks",
          headerShown: false,
          tabBarIcon: ({ focused }) => (
            <TabIcon icon={tabIcons.tasks} focused={focused} />
          ),
        }}
      />
      <Tabs.Screen
        name="memories"
        options={{
          title: "Memories",
          headerShown: false,
          tabBarIcon: ({ focused }) => (
            <TabIcon icon={tabIcons.memories} focused={focused} />
          ),
        }}
      />
      <Tabs.Screen
        name="ideas"
        options={{
          title: "Ideas",
          headerShown: false,
          tabBarIcon: ({ focused }) => (
            <TabIcon icon={tabIcons.ideas} focused={focused} />
          ),
        }}
      />
      <Tabs.Screen
        name="visceral"
        options={{
          title: "Visceral",
          tabBarIcon: ({ focused }) => (
            <TabIcon icon={tabIcons.visceral} focused={focused} />
          ),
        }}
      />
      <Tabs.Screen
        name="activity"
        options={{
          title: "Activity",
          tabBarIcon: ({ focused }) => (
            <TabIcon icon={tabIcons.activity} focused={focused} />
          ),
        }}
      />
      <Tabs.Screen
        name="recall"
        options={{
          title: "Recall",
          tabBarIcon: ({ focused }) => (
            <TabIcon icon={tabIcons.recall} focused={focused} />
          ),
        }}
      />
      <Tabs.Screen
        name="settings"
        options={{
          title: "Settings",
          tabBarIcon: ({ focused }) => (
            <TabIcon icon={tabIcons.settings} focused={focused} />
          ),
        }}
      />
    </Tabs>
  );
}

const styles = StyleSheet.create({
  icon: {
    fontSize: 20,
    lineHeight: 24,
    textAlign: "center",
  },
});
