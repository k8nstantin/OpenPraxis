/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./app/**/*.{js,jsx,ts,tsx}", "./components/**/*.{js,jsx,ts,tsx}"],
  presets: [require("nativewind/preset")],
  theme: {
    extend: {
      colors: {
        // Match OpenPraxis dashboard dark theme
        bg: {
          primary: "#0a0a0f",
          secondary: "#12121a",
          sidebar: "#0e0e16",
          card: "rgba(26, 26, 46, 0.7)",
          "card-hover": "rgba(26, 26, 46, 0.9)",
          input: "rgba(255, 255, 255, 0.05)",
        },
        text: {
          primary: "#e4e4e7",
          secondary: "#a1a1aa",
          muted: "#71717a",
        },
        accent: {
          DEFAULT: "#3b82f6",
          hover: "#60a5fa",
        },
        status: {
          success: "#00d97e",
          warning: "#f5c542",
          error: "#e63757",
        },
        border: {
          DEFAULT: "rgba(255, 255, 255, 0.08)",
          hover: "rgba(255, 255, 255, 0.15)",
        },
      },
      fontFamily: {
        sans: [
          "-apple-system",
          "BlinkMacSystemFont",
          "Segoe UI",
          "system-ui",
          "sans-serif",
        ],
        mono: ["SF Mono", "Menlo", "Monaco", "Consolas", "monospace"],
      },
      borderRadius: {
        card: "12px",
      },
    },
  },
  plugins: [],
};
