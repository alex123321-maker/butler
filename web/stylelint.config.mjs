export default {
  ignoreFiles: [
    "node_modules/**",
    ".nuxt/**",
    ".output/**",
    "dist/**",
    "app/styles/tokens.css"
  ],
  overrides: [
    {
      files: ["**/*.{vue,html}"],
      customSyntax: "postcss-html"
    }
  ],
  rules: {
    "color-no-hex": true,
    "declaration-property-value-disallowed-list": {
      "/^(color|background|background-color|border|border-color|outline|outline-color)$/": [
        "/#[0-9a-fA-F]{3,8}/"
      ]
    },
    "alpha-value-notation": null,
    "color-function-alias-notation": null,
    "color-function-notation": null
  }
};
