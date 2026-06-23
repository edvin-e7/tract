import { useCallback, useEffect, useState } from "react";

export type Theme = "ink" | "paper";

const KEY = "tract-theme";

function initial(): Theme {
  const saved = localStorage.getItem(KEY);
  return saved === "paper" ? "paper" : "ink";
}

// useTheme keeps the [data-theme] attribute, localStorage, and React state in
// lockstep. Default is "ink" (dark); "paper" is the warm light reading mode.
export function useTheme(): [Theme, () => void] {
  const [theme, setTheme] = useState<Theme>(initial);

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
    localStorage.setItem(KEY, theme);
  }, [theme]);

  const toggle = useCallback(() => setTheme((t) => (t === "ink" ? "paper" : "ink")), []);
  return [theme, toggle];
}
