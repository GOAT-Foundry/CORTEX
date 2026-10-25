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
  directive: string;
  setLocale: (loc: Locale) => void;
}

const I18nContext = createContext<I18nContextValue>({
  locale: "en",
  dict: en,
  directive: en.app.agentDirectivePreview,
  setLocale: () => {},
});

export function I18nProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>("en");
  const [serverDirective, setServerDirective] = useState<string>("");
  const reqSeqRef = React.useRef(0);

  const syncToServer = (target: Locale) => {
    const seq = ++reqSeqRef.current;
    fetch("/api/i18n", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ locale: target }),
    })
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => {
        if (seq !== reqSeqRef.current) return;
        if (data && typeof data.directive === "string") {
          setServerDirective(data.directive);
        }
      })
      .catch(() => {});
  };

  useEffect(() => {
    const saved = window.localStorage.getItem("cortex_locale");
    if (saved === "ja" || saved === "en") {
      setLocaleState(saved);
      document.documentElement.lang = saved;
      syncToServer(saved);
      return;
    }

    fetch("/api/i18n")
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => {
        if (data && (data.locale === "ja" || data.locale === "en")) {
          setLocaleState(data.locale);
          document.documentElement.lang = data.locale;
          window.localStorage.setItem("cortex_locale", data.locale);
          if (typeof data.directive === "string") {
            setServerDirective(data.directive);
          }
          return;
        }
        const detected: Locale = navigator.language
          .toLowerCase()
          .startsWith("ja")
          ? "ja"
          : "en";
        setLocaleState(detected);
        document.documentElement.lang = detected;
        syncToServer(detected);
      })
      .catch(() => {
        const detected: Locale = navigator.language
          .toLowerCase()
          .startsWith("ja")
          ? "ja"
          : "en";
        setLocaleState(detected);
        document.documentElement.lang = detected;
      });
  }, []);

  const setLocale = (next: Locale) => {
    setLocaleState(next);
    document.documentElement.lang = next;
    window.localStorage.setItem("cortex_locale", next);
    setServerDirective("");
    syncToServer(next);
  };

  const dict = dictionaries[locale];
  const directive = serverDirective || dict.app.agentDirectivePreview;

  return (
    <I18nContext.Provider value={{ locale, dict, directive, setLocale }}>
      {children}
    </I18nContext.Provider>
  );
}

export function useI18n() {
  return useContext(I18nContext);
}
