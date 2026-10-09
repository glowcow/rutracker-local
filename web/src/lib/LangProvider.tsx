import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import {
  type Ctx,
  type Lang,
  LangCtx,
  LOCALE,
  STORAGE_KEY,
  detectInitialLang,
  dicts,
  plural,
} from "./i18n";

// In its own file: react-refresh wants a component module to export only components.
export function LangProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(detectInitialLang);

  useEffect(() => {
    document.documentElement.lang = lang;
  }, [lang]);

  const setLang = useCallback((l: Lang) => {
    setLangState(l);
    try {
      localStorage.setItem(STORAGE_KEY, l);
    } catch {
      // Storage unavailable: the choice holds for this page only.
    }
  }, []);

  const t = dicts[lang];

  const locale = LOCALE[lang];
  const pluralRules = useMemo(() => new Intl.PluralRules(locale), [locale]);

  // The noun for an exact count; an abbreviated K/M figure takes pAbbr.
  const p = useCallback(
    (key: keyof typeof plural["ru"], count: number) => {
      const form = pluralRules.select(count) as keyof typeof plural.ru[typeof key];
      const forms = plural[lang][key];
      return forms[form] ?? forms.other;
    },
    [lang, pluralRules],
  );

  // "2.8M раздач" reads as a quantity with a unit: genitive plural, always.
  const pAbbr = useCallback(
    (key: keyof typeof plural["ru"]) => {
      const forms = plural[lang][key];
      return forms.many ?? forms.other;
    },
    [lang],
  );

  const value: Ctx = useMemo(
    () => ({ lang, setLang, t, locale, p, pAbbr }),
    [lang, setLang, t, locale, p, pAbbr],
  );

  return <LangCtx.Provider value={value}>{children}</LangCtx.Provider>;
}
