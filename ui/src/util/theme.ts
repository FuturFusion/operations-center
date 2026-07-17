const prefersDark = window.matchMedia("(prefers-color-scheme: dark)");

const applyTheme = () => {
  document.documentElement.setAttribute(
    "data-bs-theme",
    prefersDark.matches ? "dark" : "light",
  );
};

export const initTheme = () => {
  applyTheme();
  prefersDark.addEventListener("change", applyTheme);
};
