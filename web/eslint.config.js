export default [
  { ignores: ["dist/**", "node_modules/**"] },
  {
    files: ["**/*.js"],
    rules: {
      eqeqeq: "error",
      "no-undef": "error",
      "no-unused-vars": ["warn", { args: "none" }],
    },
  },
];
