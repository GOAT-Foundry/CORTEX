"use client";

import React, { createContext, useContext, useEffect, useState } from "react";
import en from "./en.json";
import ja from "./ja.json";

export type Locale = "en" | "ja";
type Dictionary = typeof en;

const dictionaries: Record<Locale, Dictionary> = { en, ja };

interface I18nContextValue {
  locale: Locale;
  dict: Dictionary;
  setLocale: (loc: Locale) => void;
}

const I18nContext = createContext<I18nContextValue>({
  locale: "en",
  dict: en,
  setLocale: () => {},
});

export function I18nProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>("en");

  useEffect(() => {
    const saved = window.localStorage.getItem("cortex_locale");
    if (saved === "ja" || saved === "en") {
      setLocaleState(saved);
    } else if (navigator.language.toLowerCase().startsWith("ja")) {
      setLocaleState("ja");
    }
  }, []);

  const setLocale = (next: Locale) => {
    setLocaleState(next);
    window.localStorage.setItem("cortex_locale", next);
    fetch("/api/i18n", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ locale: next }),
    }).catch(() => {});
  };

  return (
    <I18nContext.Provider
      value={{ locale, dict: dictionaries[locale], setLocale }}
    >
      {children}
    </I18nContext.Provider>
  );
}

export function useI18n() {
  return useContext(I18nContext);
}
