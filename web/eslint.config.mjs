import tsParser from "@typescript-eslint/parser";
import tsPlugin from "@typescript-eslint/eslint-plugin";
import vuePlugin from "eslint-plugin-vue";
import vueParser from "vue-eslint-parser";

const guardedFiles = [
  "app/**/*.{js,ts,vue}",
  "widgets/**/*.{js,ts,vue}",
  "features/**/*.{js,ts,vue}",
  "entities/**/*.{js,ts,vue}",
  "shared/**/*.{js,ts,vue}",
  "layouts/default.vue",
  "pages/index.vue",
  "pages/tasks.vue",
  "pages/tasks/**/*.vue",
  "pages/approvals.vue",
  "pages/artifacts.vue",
  "pages/activity.vue",
  "pages/system.vue"
];

export default [
  {
    ignores: [
      "node_modules/**",
      ".nuxt/**",
      ".output/**",
      "dist/**"
    ]
  },
  {
    files: guardedFiles,
    languageOptions: {
      parser: vueParser,
      parserOptions: {
        parser: tsParser,
        ecmaVersion: "latest",
        sourceType: "module",
        extraFileExtensions: [".vue"]
      }
    },
    plugins: {
      "@typescript-eslint": tsPlugin,
      vue: vuePlugin
    },
    rules: {
      "@typescript-eslint/no-explicit-any": "error",
      "vue/no-static-inline-styles": "error",
      "no-restricted-imports": [
        "error",
        {
          patterns: [
            {
              group: ["primevue", "primevue/*"],
              message: "Import PrimeVue only through shared/ui wrappers."
            }
          ]
        }
      ]
    }
  },
  {
    files: ["shared/ui/**/*.{js,ts,vue}"],
    rules: {
      "no-restricted-imports": "off"
    }
  }
];
