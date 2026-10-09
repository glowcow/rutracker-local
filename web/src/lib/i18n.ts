import { createContext, useContext } from "react";

// Strings the UI surfaces. Anything user-typed (forum names, titles, BBCode
// content) is never run through here — only labels we author.
export type Dict = {
  searchPlaceholder: string;
  searchClearAria: string;

  settings: string;
  theme: string;
  themeSystem: string;
  themeLight: string;
  themeDark: string;
  palette: string;
  paletteClassic: string;
  paletteWarm: string;
  language: string;
  cancel: string;

  sortLabel: string;
  sortRelevance: string;
  sortDate: string;
  sortSize: string;

  metaForum: string;
  metaSize: string;
  metaRegistered: string;
  metaHash: string;

  heroFindSomething: string;
  emptyTitle: string;
  emptyHint: string;

  searching: string;
  noResults: string;
  errorPrefix: string;
  errorGeneric: string;

  chipClearAll: string;
  chipRemoveFilter: string;

  drawerTorrent: string;
  drawerDescription: string;
  drawerLoading: string;
  drawerLoadError: string;
  drawerDownload: string;
  drawerDownloadTitle: string;
  drawerTopic: string;
  drawerTopicAria: string;
  drawerTopicTitle: string;
  drawerMagnetTitle: string;
  drawerMagnetCopied: string;
  drawerTransmission: string;
  drawerTransmissionTitle: string;
  drawerTransmissionOffline: string;
  drawerTransmissionAdded: string;
  drawerTransmissionDuplicate: string;
  drawerTransmissionError: string;
  drawerClose: string;
  drawerFiles: string;
  drawerFilesLoading: string;
  drawerFilesError: string;
  drawerFilesTruncated: (n: number) => string;
  drawerFilesNone: string;

  forumsDirect: string;
  forumsUmbrella: string;
  forumsShow: string;
  forumsHide: string;

  favoritesToggleAria: string;
  favoritesAddAria: string;
  favoritesRemoveAria: string;
  favoritesTitle: string;
  favoritesEmptyTitle: string;
  favoritesEmptyHint: string;
  favoritesClearAll: string;
  favoritesClearConfirm: string;

  paginationAria: string;
  paginationPrev: string;
  paginationNext: string;
  paginationPage: string;

  footerBuilt: string;

  adminOpenAria: string;
  adminTitle: string;
  adminSubtitle: string;
  adminClose: string;
  adminDump: string;
  adminNoDumps: string;
  adminBatch: string;
  adminSweep: string;
  adminSweepHint: string;
  adminStart: string;
  adminRunning: string;
  adminToken: string;
  adminTokenPlaceholder: string;
  adminTokenSave: string;
  adminTokenSaved: string;
  adminTokenChange: string;
  adminTokenRequired: string;
  adminStatusIdle: string;
  adminStatusRunning: string;
  adminStatusSucceeded: string;
  adminStatusFailed: string;
  adminRows: string;
  adminRate: string;
  adminEta: string;
  adminElapsed: string;
  adminLogs: string;
  adminRecent: string;
  adminNoRuns: string;
  adminErrConflict: string;
  adminErrUnauthorized: string;
  adminErrDisabled: string;
  adminErrGeneric: string;

  // Peer stats (seeders / leechers) — live layer scraped from rutracker.
  peersLabel: string;
  peersSeeders: string;
  peersLeechers: string;
  peersUpdated: string;
  peersNow: string;
  peersAgo: string;
  peersMin: string;
  peersHour: string;
  peersDay: string;
  peersStale: string;
  peersErrAuth: string;
  peersErrUnavailable: string;
  peersLoading: string;
  peersCachedLabel: string;

  // Tooltip on the stats ribbon's "@ dd-mm-yyyy" dump freshness stamp.
  statsDumpUpdated: string;
};

// Pluralisation helpers. "results" / "torrents" / "forums" each get a verb
// form chosen by Intl.PluralRules; Russian has one/few/many, English one/other.
export type PluralForms = { one: string; few?: string; many?: string; other: string };

const ru: Dict = {
  searchPlaceholder: "Найти раздачу…",
  searchClearAria: "Очистить поиск",

  settings: "Настройки",
  theme: "Тема",
  themeSystem: "Системная",
  themeLight: "Светлая",
  themeDark: "Тёмная",
  palette: "Цветовая схема",
  paletteClassic: "Классическая",
  paletteWarm: "Тёплая",
  language: "Язык",
  cancel: "Отмена",

  sortLabel: "Сортировка",
  sortRelevance: "Релевантность",
  sortDate: "Дата",
  sortSize: "Размер",

  metaForum: "Форум",
  metaSize: "Размер",
  metaRegistered: "Добавлено",
  metaHash: "Хэш",

  heroFindSomething: "Найди что-нибудь интересное",
  emptyTitle: "Начни с поиска или выбери форум",
  emptyHint: "Введи название раздачи сверху, либо ткни в один из форумов в шапке.",

  searching: "Ищем…",
  noResults: "Ничего не нашлось — попробуй упростить запрос",
  errorPrefix: "Ошибка: ",
  errorGeneric: "что-то пошло не так",

  chipClearAll: "Очистить",
  chipRemoveFilter: "Снять фильтр",

  drawerTorrent: "Раздача",
  drawerDescription: "Описание",
  drawerLoading: "Загружается…",
  drawerLoadError: "Не удалось загрузить раздачу",
  drawerDownload: "Скачать",
  drawerTopic: "RuTracker",
  drawerTopicAria: "Открыть тему на rutracker.org",
  drawerTopicTitle: "Открыть тему на rutracker.org",
  drawerDownloadTitle: "Скачать .torrent через magnet",
  drawerMagnetTitle: "Скопировать magnet-ссылку",
  drawerMagnetCopied: "Скопировано",
  drawerTransmission: "Transmission",
  drawerTransmissionTitle: "Отправить на закачку в Transmission",
  drawerTransmissionOffline: "Transmission недоступен",
  drawerTransmissionAdded: "Добавлено",
  drawerTransmissionDuplicate: "Уже в очереди",
  drawerTransmissionError: "Ошибка",
  drawerClose: "Закрыть",
  drawerFiles: "Файлы",
  drawerFilesLoading: "Загружается список файлов…",
  drawerFilesError: "Не удалось загрузить список файлов",
  drawerFilesTruncated: (n) => `показана первая 1000, ещё ${n}`,
  drawerFilesNone: "Дамп не содержит списка файлов для этой раздачи",

  forumsDirect: "Прямые форумы",
  forumsUmbrella: "Общий",
  forumsShow: "Показать форумы",
  forumsHide: "Скрыть форумы",

  favoritesToggleAria: "Избранное",
  favoritesAddAria: "В избранное",
  favoritesRemoveAria: "Убрать из избранного",
  favoritesTitle: "Избранное",
  favoritesEmptyTitle: "Здесь будет твоё избранное",
  favoritesEmptyHint: "Нажми звёздочку на любой раздаче, чтобы добавить.",
  favoritesClearAll: "Очистить избранное",
  favoritesClearConfirm: "Все раздачи будут убраны из избранного. Отменить это нельзя.",

  paginationAria: "Постраничная навигация",
  paginationPrev: "Предыдущая страница",
  paginationNext: "Следующая страница",
  paginationPage: "стр.",

  footerBuilt: "собрано",

  adminOpenAria: "Загрузка дампа",
  adminTitle: "Парсер",
  adminSubtitle: "Загрузка дампа",
  adminClose: "Закрыть",
  adminDump: "Дамп",
  adminNoDumps: "Дампы не найдены",
  adminBatch: "Размер батча",
  adminSweep: "Sweep",
  adminSweepHint: "Удалит раздачи, которых нет в новом дампе. Только для полного дампа.",
  adminStart: "Запустить парсинг",
  adminRunning: "Идёт парсинг…",
  adminToken: "Токен доступа",
  adminTokenPlaceholder: "Вставь admin-токен",
  adminTokenSave: "Сохранить",
  adminTokenSaved: "Токен сохранён",
  adminTokenChange: "Сменить",
  adminTokenRequired: "Введите токен, чтобы запустить",
  adminStatusIdle: "Ожидание",
  adminStatusRunning: "Выполняется",
  adminStatusSucceeded: "Готово",
  adminStatusFailed: "Ошибка",
  adminRows: "строк",
  adminRate: "строк/с",
  adminEta: "осталось",
  adminElapsed: "прошло",
  adminLogs: "Логи",
  adminRecent: "Последние загрузки",
  adminNoRuns: "Пока не было загрузок",
  adminErrConflict: "Парсинг уже идёт",
  adminErrUnauthorized: "Неверный токен",
  adminErrDisabled: "Ручка отключена на сервере (нет токена)",
  adminErrGeneric: "Не удалось запустить",

  peersLabel: "Пиры",
  peersSeeders: "Сиды",
  peersLeechers: "Личи",
  peersUpdated: "обновлено",
  peersNow: "только что",
  peersAgo: "назад",
  peersMin: "мин",
  peersHour: "ч",
  peersDay: "дн",
  peersStale: "не удалось обновить",
  peersErrAuth: "не удалось авторизоваться",
  peersErrUnavailable: "rutracker недоступен",
  peersLoading: "проверяем…",
  peersCachedLabel: "с пирами",

  statsDumpUpdated: "Дамп обновлён",
};

const en: Dict = {
  searchPlaceholder: "Find a torrent…",
  searchClearAria: "Clear search",

  settings: "Settings",
  theme: "Theme",
  themeSystem: "System",
  themeLight: "Light",
  themeDark: "Dark",
  palette: "Colour scheme",
  paletteClassic: "Classic",
  paletteWarm: "Warm",
  language: "Language",
  cancel: "Cancel",

  sortLabel: "Sort",
  sortRelevance: "Relevance",
  sortDate: "Date",
  sortSize: "Size",

  metaForum: "Forum",
  metaSize: "Size",
  // Abbreviated: the full "Registered" is the widest label in the drawer's
  // meta grid and pushed every value right by ~40px.
  metaRegistered: "Reg.",
  metaHash: "Hash",

  heroFindSomething: "Find something interesting",
  emptyTitle: "Start with a search or pick a forum",
  emptyHint: "Type a torrent name above, or tap a forum in the header.",

  searching: "Searching…",
  noResults: "Nothing matched — try a simpler query",
  errorPrefix: "Error: ",
  errorGeneric: "something went wrong",

  chipClearAll: "Clear",
  chipRemoveFilter: "Remove filter",

  drawerTorrent: "Torrent",
  drawerDescription: "Description",
  drawerLoading: "Loading…",
  drawerLoadError: "Couldn't load this torrent",
  drawerDownload: "Download",
  drawerTopic: "RuTracker",
  drawerTopicAria: "Open topic on rutracker.org",
  drawerTopicTitle: "Open topic on rutracker.org",
  drawerDownloadTitle: "Download .torrent via magnet",
  drawerMagnetTitle: "Copy magnet link",
  drawerMagnetCopied: "Copied",
  drawerTransmission: "Transmission",
  drawerTransmissionTitle: "Send to Transmission download queue",
  drawerTransmissionOffline: "Transmission unavailable",
  drawerTransmissionAdded: "Added",
  drawerTransmissionDuplicate: "Already queued",
  drawerTransmissionError: "Error",
  drawerClose: "Close",
  drawerFiles: "Files",
  drawerFilesLoading: "Loading file list…",
  drawerFilesError: "Could not load the file list",
  drawerFilesTruncated: (n) => `first 1000 shown, ${n} more`,
  drawerFilesNone: "The dump carries no file list for this torrent",

  forumsDirect: "Direct forums",
  forumsUmbrella: "General",
  forumsShow: "Show forums",
  forumsHide: "Hide forums",

  favoritesToggleAria: "Favourites",
  favoritesAddAria: "Add to favourites",
  favoritesRemoveAria: "Remove from favourites",
  favoritesTitle: "Favourites",
  favoritesEmptyTitle: "Your favourites will live here",
  favoritesEmptyHint: "Tap the star on any torrent to add it.",
  favoritesClearAll: "Clear favourites",
  favoritesClearConfirm: "Every torrent leaves the favourites. This cannot be undone.",

  paginationAria: "Pagination",
  paginationPrev: "Previous page",
  paginationNext: "Next page",
  paginationPage: "page",

  footerBuilt: "built",

  adminOpenAria: "Load dump",
  adminTitle: "Parser",
  adminSubtitle: "Dump load",
  adminClose: "Close",
  adminDump: "Dump",
  adminNoDumps: "No dumps found",
  adminBatch: "Batch size",
  adminSweep: "Sweep",
  adminSweepHint: "Deletes torrents missing from the new dump. Full dumps only.",
  adminStart: "Start parse",
  adminRunning: "Parsing…",
  adminToken: "Access token",
  adminTokenPlaceholder: "Paste admin token",
  adminTokenSave: "Save",
  adminTokenSaved: "Token saved",
  adminTokenChange: "Change",
  adminTokenRequired: "Enter a token to start",
  adminStatusIdle: "Idle",
  adminStatusRunning: "Running",
  adminStatusSucceeded: "Done",
  adminStatusFailed: "Failed",
  adminRows: "rows",
  adminRate: "rows/s",
  adminEta: "ETA",
  adminElapsed: "elapsed",
  adminLogs: "Logs",
  adminRecent: "Recent loads",
  adminNoRuns: "No loads yet",
  adminErrConflict: "A parse is already running",
  adminErrUnauthorized: "Invalid token",
  adminErrDisabled: "Endpoint disabled on the server (no token)",
  adminErrGeneric: "Couldn't start",

  peersLabel: "Peers",
  peersSeeders: "Seeders",
  peersLeechers: "Leechers",
  peersUpdated: "updated",
  peersNow: "just now",
  peersAgo: "ago",
  peersMin: "m",
  peersHour: "h",
  peersDay: "d",
  peersStale: "couldn't refresh",
  peersErrAuth: "authorization failed",
  peersErrUnavailable: "rutracker unavailable",
  peersLoading: "checking…",
  peersCachedLabel: "with peers",

  statsDumpUpdated: "Dump updated",
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
  /** The current dictionary; a string with a number in it is a function. */
  t: Dict;
  /** What numbers and dates are formatted for. */
  locale: string;
  // The noun for an exact count, picked by Intl.PluralRules.
  p: (key: keyof typeof plural["ru"], count: number) => string;
  // The noun for an abbreviated count ("2.8M раздач"): genitive plural.
  pAbbr: (key: keyof typeof plural["ru"]) => string;
};

// The provider is in LangProvider.tsx: react-refresh wants a component
// module to export only components.
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
