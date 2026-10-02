import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import {
  type Ctx,
  type Dict,
  type Lang,
  LangCtx,
  STORAGE_KEY,
  detectInitialLang,
  dicts,
  plural,
} from "./i18n";

// LangProvider lives in its own file because eslint-plugin-react-refresh
// requires that any module exporting a component export ONLY components.
// All non-component exports (useLang hook, dicts, types) stay in i18n.ts.
export function LangProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(detectInitialLang);

  useEffect(() => {
    document.documentElement.lang = lang;
  }, [lang]);

  const setLang = useCallback((l: Lang) => {
    setLangState(l);
    localStorage.setItem(STORAGE_KEY, l);
  }, []);

  const t = useCallback((k: keyof Dict) => dicts[lang][k], [lang]);

  // PluralRules constructor is non-trivial — cache per lang.
  const pluralRules = useMemo(
    () => new Intl.PluralRules(lang === "ru" ? "ru-RU" : "en-US"),
    [lang],
  );

  // p: pure Intl.PluralRules — correct for EXACT displayed counts. The old
  // `count >= 1000 → many` shortcut was written for the abbreviated K/M
  // stats ribbon but leaked into the meta-row, producing "1 021 результатов"
  // instead of "1 021 результат". Abbreviated call sites use pAbbr instead.
  const p = useCallback(
    (key: keyof typeof plural["ru"], count: number) => {
      const form = pluralRules.select(count) as keyof typeof plural.ru[typeof key];
      const forms = plural[lang][key];
      return forms[form] ?? forms.other;
    },
    [lang, pluralRules],
  );

  // pAbbr: noun for abbreviated K/M phrases. Russian reads "2.8M раздач" as
  // quantity-with-unit ("2.8 миллиона раздач") — genitive plural ("many")
  // regardless of the exact number's trailing digit.
  const pAbbr = useCallback(
    (key: keyof typeof plural["ru"]) => {
      const forms = plural[lang][key];
      return forms.many ?? forms.other;
    },
    [lang],
  );

  const value: Ctx = useMemo(
    () => ({ lang, setLang, t, p, pAbbr }),
    [lang, setLang, t, p, pAbbr],
  );

  return <LangCtx.Provider value={value}>{children}</LangCtx.Provider>;
}
