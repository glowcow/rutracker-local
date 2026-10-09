import { createContext, useContext } from "react";

// Strings the UI surfaces. Anything user-typed (forum names, titles, BBCode
// content) is never run through here — only labels we author.
export type Dict = {
  search_placeholder: string;
  search_clear_aria: string;

  settings: string;
  theme: string;
  theme_system: string;
  theme_light: string;
  theme_dark: string;
  palette: string;
  palette_classic: string;
  palette_warm: string;
  language: string;
  cancel: string;

  sort_label: string;
  sort_relevance: string;
  sort_date: string;
  sort_size: string;

  meta_forum: string;
  meta_size: string;
  meta_registered: string;
  meta_hash: string;

  hero_find_something: string;
  empty_title: string;
  empty_hint: string;

  searching: string;
  no_results: string;
  error_prefix: string;
  error_generic: string;

  chip_clear_all: string;
  chip_remove_filter: string;

  drawer_torrent: string;
  drawer_description: string;
  drawer_loading: string;
  drawer_load_error: string;
  drawer_download: string;
  drawer_download_title: string;
  drawer_topic: string;
  drawer_topic_aria: string;
  drawer_topic_title: string;
  drawer_magnet_title: string;
  drawer_magnet_copied: string;
  drawer_transmission: string;
  drawer_transmission_title: string;
  drawer_transmission_offline: string;
  drawer_transmission_added: string;
  drawer_transmission_duplicate: string;
  drawer_transmission_error: string;
  drawer_close: string;
  drawer_files: string;
  drawer_files_loading: string;
  drawer_files_error: string;
  drawer_files_truncated: string;
  drawer_files_none: string;

  forums_direct: string;
  forums_umbrella: string;
  forums_show: string;
  forums_hide: string;

  favorites_toggle_aria: string;
  favorites_add_aria: string;
  favorites_remove_aria: string;
  favorites_title: string;
  favorites_empty_title: string;
  favorites_empty_hint: string;
  favorites_clear_all: string;
  favorites_clear_confirm: string;

  pagination_aria: string;
  pagination_prev: string;
  pagination_next: string;
  pagination_page: string;

  footer_built: string;

  admin_open_aria: string;
  admin_title: string;
  admin_subtitle: string;
  admin_close: string;
  admin_dump: string;
  admin_no_dumps: string;
  admin_batch: string;
  admin_sweep: string;
  admin_sweep_hint: string;
  admin_start: string;
  admin_running: string;
  admin_token: string;
  admin_token_placeholder: string;
  admin_token_save: string;
  admin_token_saved: string;
  admin_token_change: string;
  admin_token_required: string;
  admin_status_idle: string;
  admin_status_running: string;
  admin_status_succeeded: string;
  admin_status_failed: string;
  admin_rows: string;
  admin_rate: string;
  admin_eta: string;
  admin_elapsed: string;
  admin_logs: string;
  admin_recent: string;
  admin_no_runs: string;
  admin_err_conflict: string;
  admin_err_unauthorized: string;
  admin_err_disabled: string;
  admin_err_generic: string;

  // Peer stats (seeders / leechers) — live layer scraped from rutracker.
  peers_label: string;
  peers_seeders: string;
  peers_leechers: string;
  peers_updated: string;
  peers_now: string;
  peers_ago: string;
  peers_min: string;
  peers_hour: string;
  peers_day: string;
  peers_stale: string;
  peers_err_auth: string;
  peers_err_unavailable: string;
  peers_loading: string;
  peers_cached_label: string;

  // Tooltip on the stats ribbon's "@ dd-mm-yyyy" dump freshness stamp.
  stats_dump_updated: string;
};

// Pluralisation helpers. "results" / "torrents" / "forums" each get a verb
// form chosen by Intl.PluralRules; Russian has one/few/many, English one/other.
export type PluralForms = { one: string; few?: string; many?: string; other: string };

const ru: Dict = {
  search_placeholder: "Найти раздачу…",
  search_clear_aria: "Очистить поиск",

  settings: "Настройки",
  theme: "Тема",
  theme_system: "Системная",
  theme_light: "Светлая",
  theme_dark: "Тёмная",
  palette: "Цветовая схема",
  palette_classic: "Классическая",
  palette_warm: "Тёплая",
  language: "Язык",
  cancel: "Отмена",

  sort_label: "Сортировка",
  sort_relevance: "Релевантность",
  sort_date: "Дата",
  sort_size: "Размер",

  meta_forum: "Форум",
  meta_size: "Размер",
  meta_registered: "Добавлено",
  meta_hash: "Хэш",

  hero_find_something: "Найди что-нибудь интересное",
  empty_title: "Начни с поиска или выбери форум",
  empty_hint: "Введи название раздачи сверху, либо ткни в один из форумов в шапке.",

  searching: "Ищем…",
  no_results: "Ничего не нашлось — попробуй упростить запрос",
  error_prefix: "Ошибка: ",
  error_generic: "что-то пошло не так",

  chip_clear_all: "Очистить",
  chip_remove_filter: "Снять фильтр",

  drawer_torrent: "Раздача",
  drawer_description: "Описание",
  drawer_loading: "Загружается…",
  drawer_load_error: "Не удалось загрузить раздачу",
  drawer_download: "Скачать",
  drawer_topic: "RuTracker",
  drawer_topic_aria: "Открыть тему на rutracker.org",
  drawer_topic_title: "Открыть тему на rutracker.org",
  drawer_download_title: "Скачать .torrent через magnet",
  drawer_magnet_title: "Скопировать magnet-ссылку",
  drawer_magnet_copied: "Скопировано",
  drawer_transmission: "Transmission",
  drawer_transmission_title: "Отправить на закачку в Transmission",
  drawer_transmission_offline: "Transmission недоступен",
  drawer_transmission_added: "Добавлено",
  drawer_transmission_duplicate: "Уже в очереди",
  drawer_transmission_error: "Ошибка",
  drawer_close: "Закрыть",
  drawer_files: "Файлы",
  drawer_files_loading: "Загружается список файлов…",
  drawer_files_error: "Не удалось загрузить список файлов",
  // {n} — сколько файлов не поместилось в ответ (список режется на 1000).
  drawer_files_truncated: "показана первая 1000, ещё {n}",
  drawer_files_none: "Дамп не содержит списка файлов для этой раздачи",

  forums_direct: "Прямые форумы",
  forums_umbrella: "Общий",
  forums_show: "Показать форумы",
  forums_hide: "Скрыть форумы",

  favorites_toggle_aria: "Избранное",
  favorites_add_aria: "В избранное",
  favorites_remove_aria: "Убрать из избранного",
  favorites_title: "Избранное",
  favorites_empty_title: "Здесь будет твоё избранное",
  favorites_empty_hint: "Нажми звёздочку на любой раздаче, чтобы добавить.",
  favorites_clear_all: "Очистить избранное",
  favorites_clear_confirm: "Все раздачи будут убраны из избранного. Отменить это нельзя.",

  pagination_aria: "Постраничная навигация",
  pagination_prev: "Предыдущая страница",
  pagination_next: "Следующая страница",
  pagination_page: "стр.",

  footer_built: "собрано",

  admin_open_aria: "Загрузка дампа",
  admin_title: "Парсер",
  admin_subtitle: "Загрузка дампа",
  admin_close: "Закрыть",
  admin_dump: "Дамп",
  admin_no_dumps: "Дампы не найдены",
  admin_batch: "Размер батча",
  admin_sweep: "Sweep",
  admin_sweep_hint: "Удалит раздачи, которых нет в новом дампе. Только для полного дампа.",
  admin_start: "Запустить парсинг",
  admin_running: "Идёт парсинг…",
  admin_token: "Токен доступа",
  admin_token_placeholder: "Вставь admin-токен",
  admin_token_save: "Сохранить",
  admin_token_saved: "Токен сохранён",
  admin_token_change: "Сменить",
  admin_token_required: "Введите токен, чтобы запустить",
  admin_status_idle: "Ожидание",
  admin_status_running: "Выполняется",
  admin_status_succeeded: "Готово",
  admin_status_failed: "Ошибка",
  admin_rows: "строк",
  admin_rate: "строк/с",
  admin_eta: "осталось",
  admin_elapsed: "прошло",
  admin_logs: "Логи",
  admin_recent: "Последние загрузки",
  admin_no_runs: "Пока не было загрузок",
  admin_err_conflict: "Парсинг уже идёт",
  admin_err_unauthorized: "Неверный токен",
  admin_err_disabled: "Ручка отключена на сервере (нет токена)",
  admin_err_generic: "Не удалось запустить",

  peers_label: "Пиры",
  peers_seeders: "Сиды",
  peers_leechers: "Личи",
  peers_updated: "обновлено",
  peers_now: "только что",
  peers_ago: "назад",
  peers_min: "мин",
  peers_hour: "ч",
  peers_day: "дн",
  peers_stale: "не удалось обновить",
  peers_err_auth: "не удалось авторизоваться",
  peers_err_unavailable: "rutracker недоступен",
  peers_loading: "проверяем…",
  peers_cached_label: "с пирами",

  stats_dump_updated: "Дамп обновлён",
};

const en: Dict = {
  search_placeholder: "Find a torrent…",
  search_clear_aria: "Clear search",

  settings: "Settings",
  theme: "Theme",
  theme_system: "System",
  theme_light: "Light",
  theme_dark: "Dark",
  palette: "Colour scheme",
  palette_classic: "Classic",
  palette_warm: "Warm",
  language: "Language",
  cancel: "Cancel",

  sort_label: "Sort",
  sort_relevance: "Relevance",
  sort_date: "Date",
  sort_size: "Size",

  meta_forum: "Forum",
  meta_size: "Size",
  // Abbreviated: the full "Registered" is the widest label in the drawer's
  // meta grid and pushed every value right by ~40px.
  meta_registered: "Reg.",
  meta_hash: "Hash",

  hero_find_something: "Find something interesting",
  empty_title: "Start with a search or pick a forum",
  empty_hint: "Type a torrent name above, or tap a forum in the header.",

  searching: "Searching…",
  no_results: "Nothing matched — try a simpler query",
  error_prefix: "Error: ",
  error_generic: "something went wrong",

  chip_clear_all: "Clear",
  chip_remove_filter: "Remove filter",

  drawer_torrent: "Torrent",
  drawer_description: "Description",
  drawer_loading: "Loading…",
  drawer_load_error: "Couldn't load this torrent",
  drawer_download: "Download",
  drawer_topic: "RuTracker",
  drawer_topic_aria: "Open topic on rutracker.org",
  drawer_topic_title: "Open topic on rutracker.org",
  drawer_download_title: "Download .torrent via magnet",
  drawer_magnet_title: "Copy magnet link",
  drawer_magnet_copied: "Copied",
  drawer_transmission: "Transmission",
  drawer_transmission_title: "Send to Transmission download queue",
  drawer_transmission_offline: "Transmission unavailable",
  drawer_transmission_added: "Added",
  drawer_transmission_duplicate: "Already queued",
  drawer_transmission_error: "Error",
  drawer_close: "Close",
  drawer_files: "Files",
  drawer_files_loading: "Loading file list…",
  drawer_files_error: "Could not load the file list",
  drawer_files_truncated: "first 1000 shown, {n} more",
  drawer_files_none: "The dump carries no file list for this torrent",

  forums_direct: "Direct forums",
  forums_umbrella: "General",
  forums_show: "Show forums",
  forums_hide: "Hide forums",

  favorites_toggle_aria: "Favourites",
  favorites_add_aria: "Add to favourites",
  favorites_remove_aria: "Remove from favourites",
  favorites_title: "Favourites",
  favorites_empty_title: "Your favourites will live here",
  favorites_empty_hint: "Tap the star on any torrent to add it.",
  favorites_clear_all: "Clear favourites",
  favorites_clear_confirm: "Every torrent leaves the favourites. This cannot be undone.",

  pagination_aria: "Pagination",
  pagination_prev: "Previous page",
  pagination_next: "Next page",
  pagination_page: "page",

  footer_built: "built",

  admin_open_aria: "Load dump",
  admin_title: "Parser",
  admin_subtitle: "Dump load",
  admin_close: "Close",
  admin_dump: "Dump",
  admin_no_dumps: "No dumps found",
  admin_batch: "Batch size",
  admin_sweep: "Sweep",
  admin_sweep_hint: "Deletes torrents missing from the new dump. Full dumps only.",
  admin_start: "Start parse",
  admin_running: "Parsing…",
  admin_token: "Access token",
  admin_token_placeholder: "Paste admin token",
  admin_token_save: "Save",
  admin_token_saved: "Token saved",
  admin_token_change: "Change",
  admin_token_required: "Enter a token to start",
  admin_status_idle: "Idle",
  admin_status_running: "Running",
  admin_status_succeeded: "Done",
  admin_status_failed: "Failed",
  admin_rows: "rows",
  admin_rate: "rows/s",
  admin_eta: "ETA",
  admin_elapsed: "elapsed",
  admin_logs: "Logs",
  admin_recent: "Recent loads",
  admin_no_runs: "No loads yet",
  admin_err_conflict: "A parse is already running",
  admin_err_unauthorized: "Invalid token",
  admin_err_disabled: "Endpoint disabled on the server (no token)",
  admin_err_generic: "Couldn't start",

  peers_label: "Peers",
  peers_seeders: "Seeders",
  peers_leechers: "Leechers",
  peers_updated: "updated",
  peers_now: "just now",
  peers_ago: "ago",
  peers_min: "m",
  peers_hour: "h",
  peers_day: "d",
  peers_stale: "couldn't refresh",
  peers_err_auth: "authorization failed",
  peers_err_unavailable: "rutracker unavailable",
  peers_loading: "checking…",
  peers_cached_label: "with peers",

  stats_dump_updated: "Dump updated",
};

export const dicts = { en, ru };

export type Lang = "en" | "ru";

/** Each language in its own name, in menu order. */
export const LANGS: { id: Lang; name: string }[] = [
  { id: "en", name: "English" },
  { id: "ru", name: "Русский" },
];

/** What numbers and dates are formatted for. */
export const LOCALE: Record<Lang, string> = { en: "en-GB", ru: "ru-RU" };

export const plural: Record<Lang, Record<string, PluralForms>> = {
  ru: {
    results:  { one: "результат",  few: "результата",  many: "результатов",  other: "результата" },
    torrents: { one: "раздача",    few: "раздачи",     many: "раздач",       other: "раздачи" },
    forums:   { one: "форум",      few: "форума",      many: "форумов",      other: "форума" },
  },
  en: {
    results:  { one: "result",   other: "results" },
    torrents: { one: "torrent",  other: "torrents" },
    forums:   { one: "forum",    other: "forums" },
  },
};

export type Ctx = {
  lang: Lang;
  setLang: (l: Lang) => void;
  t: (k: keyof Dict) => string;
  /** What numbers and dates are formatted for. */
  locale: string;
  // Returns the noun form for `count` in the current language. The `key`
  // selects which noun ("torrents", "results", "forums"). Use with EXACT
  // displayed counts — Intl.PluralRules picks by the trailing digits.
  p: (key: keyof typeof plural["ru"], count: number) => string;
  // Noun form for ABBREVIATED counts ("2.8M раздач", "1.3K форумов"): the
  // K/M phrase reads as quantity-with-unit, which in Russian takes the
  // genitive plural regardless of the exact number's trailing digit.
  pAbbr: (key: keyof typeof plural["ru"]) => string;
};

// The Provider component lives in LangProvider.tsx: react-refresh forbids mixing
// a component export with hook/util exports in one file (breaks Fast Refresh HMR).
// This file keeps the data/types/hook; the provider next door depends on it.
export const LangCtx = createContext<Ctx | null>(null);

export const STORAGE_KEY = "lang";

/** Stored choice, else the browser's language when the app has it, else English. */
export function detectInitialLang(): Lang {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === "en" || stored === "ru") return stored;
  } catch {
    // No storage: fall through to the browser's language.
  }
  return navigator.language.toLowerCase().startsWith("ru") ? "ru" : "en";
}

export function useLang(): Ctx {
  const v = useContext(LangCtx);
  if (!v) throw new Error("useLang must be used inside <LangProvider>");
  return v;
}
