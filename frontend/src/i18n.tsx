import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";

// Lightweight i18n for a global audience. English is the default; a picker in the
// qbar switches language and persists the choice. No runtime dependency — a flat
// key→string map per language plus a {var}-interpolating translator, mirroring the
// clipboard-manager i18n pattern. An unwired key falls back to English, then to the
// key itself, so a missing string is never a crash.

export type Lang = "en" | "sv";

export const LANGS: { code: Lang; label: string }[] = [
  { code: "en", label: "English" },
  { code: "sv", label: "Svenska" },
];

// Intl locale used for date formatting per language. English keeps en-US so dates
// render exactly as before the picker existed; Swedish gets sv-SE.
export const LOCALES: Record<Lang, string> = { en: "en-US", sv: "sv-SE" };

export const DEFAULT_LANG: Lang = "en";

type Vars = Record<string, string | number>;
type Dict = Record<string, string>;

const en: Dict = {
  // library chrome
  "search.placeholder": "Search everything you've saved…",
  "search.aria": "Search library",
  "theme.toggle": "Toggle theme",
  "lang.aria": "Language",
  "filter.all": "All",
  "filter.highlighted": "Highlighted",
  "save.link": "Save a link",
  "add.placeholder": "Paste a URL to save and read clean…",
  "add.aria": "URL to save",
  "add.saving": "Saving…",
  "add.save": "Save",
  // sections
  "sec.continue": "Continue reading",
  "sec.continueSub": "pick up where you left off",
  "sec.results": "Results",
  "sec.upnext": "Up next",
  "progress.in": "{n}% in",
  // counts — {n}-inclusive
  "count.article.one": "{n} article",
  "count.article.other": "{n} articles",
  "count.highlight.one": "{n} highlight",
  "count.highlight.other": "{n} highlights",
  // counts — word only (number rendered separately)
  "noun.article.one": "article",
  "noun.article.other": "articles",
  "noun.highlight.one": "highlight",
  "noun.highlight.other": "highlights",
  // empty states
  "empty.search": "Nothing matches that search.",
  "empty.highlighted": "No highlighted articles yet.",
  "empty.library": "Nothing saved yet — paste a link to begin your library.",
  "delete.aria": "Delete {title}",
  // footer
  "foot.search": "search",
  "foot.open": "open",
  // relative time
  "time.justnow": "just now",
  "time.min": "{n} min ago",
  "time.hr.one": "{n} hr ago",
  "time.hr.other": "{n} hrs ago",
  "time.day.one": "{n} day ago",
  "time.day.other": "{n} days ago",
  // reader
  "reader.back": "Library",
  "reader.copied": "Copied",
  "reader.copyLink": "Copy link",
  "reader.highlights": "Highlights",
  "reader.hlEmpty":
    "Select any passage in the article to highlight it — your marks are saved and come back every time you open it.",
  "reader.jump": "Jump to highlight",
  "reader.remove": "Remove highlight",
  "reader.highlight": "Highlight",
  "common.loading": "Loading…",
  // access token (protected deploys)
  "token.aria": "Access token",
  "token.label": "Access token",
  "token.set": "Access token — set",
  "token.placeholder": "Paste access token…",
  "token.hint": "Needed to save or delete on a protected server. Kept only in this browser.",
  "token.save": "Save",
  "token.clear": "Clear",
  // error fallbacks (network message, when present, stays as returned)
  "err.load": "load failed",
  "err.saveLink": "could not save link",
  "err.delete": "could not delete",
  "err.saveHl": "could not save highlight",
  "err.removeHl": "could not remove highlight",
  "err.unauthorized": "This server requires an access token — set it with the key button.",
};

const sv: Dict = {
  "search.placeholder": "Sök i allt du sparat…",
  "search.aria": "Sök i biblioteket",
  "theme.toggle": "Växla tema",
  "lang.aria": "Språk",
  "filter.all": "Alla",
  "filter.highlighted": "Markerade",
  "save.link": "Spara en länk",
  "add.placeholder": "Klistra in en URL att spara och läsa rent…",
  "add.aria": "URL att spara",
  "add.saving": "Sparar…",
  "add.save": "Spara",
  "sec.continue": "Fortsätt läsa",
  "sec.continueSub": "fortsätt där du slutade",
  "sec.results": "Resultat",
  "sec.upnext": "Näst på tur",
  "progress.in": "{n}% läst",
  "count.article.one": "{n} artikel",
  "count.article.other": "{n} artiklar",
  "count.highlight.one": "{n} markering",
  "count.highlight.other": "{n} markeringar",
  "noun.article.one": "artikel",
  "noun.article.other": "artiklar",
  "noun.highlight.one": "markering",
  "noun.highlight.other": "markeringar",
  "empty.search": "Inget matchar sökningen.",
  "empty.highlighted": "Inga markerade artiklar än.",
  "empty.library": "Inget sparat än — klistra in en länk för att börja ditt bibliotek.",
  "delete.aria": "Ta bort {title}",
  "foot.search": "sök",
  "foot.open": "öppna",
  "time.justnow": "nyss",
  "time.min": "{n} min sedan",
  "time.hr.one": "{n} tim sedan",
  "time.hr.other": "{n} tim sedan",
  "time.day.one": "{n} dag sedan",
  "time.day.other": "{n} dagar sedan",
  "reader.back": "Biblioteket",
  "reader.copied": "Kopierad",
  "reader.copyLink": "Kopiera länk",
  "reader.highlights": "Markeringar",
  "reader.hlEmpty":
    "Markera valfritt stycke i artikeln för att spara det — dina markeringar sparas och kommer tillbaka varje gång du öppnar den.",
  "reader.jump": "Hoppa till markering",
  "reader.remove": "Ta bort markering",
  "reader.highlight": "Markera",
  "common.loading": "Laddar…",
  "token.aria": "Åtkomsttoken",
  "token.label": "Åtkomsttoken",
  "token.set": "Åtkomsttoken — angiven",
  "token.placeholder": "Klistra in åtkomsttoken…",
  "token.hint": "Krävs för att spara eller ta bort på en skyddad server. Sparas bara i den här webbläsaren.",
  "token.save": "Spara",
  "token.clear": "Rensa",
  "err.load": "kunde inte ladda",
  "err.saveLink": "kunde inte spara länken",
  "err.delete": "kunde inte ta bort",
  "err.saveHl": "kunde inte spara markeringen",
  "err.removeHl": "kunde inte ta bort markeringen",
  "err.unauthorized": "Servern kräver en åtkomsttoken — ange den med nyckelknappen.",
};

const DICTS: Record<Lang, Dict> = { en, sv };

function interpolate(s: string, vars?: Vars): string {
  if (!vars) return s;
  return s.replace(/\{(\w+)\}/g, (_, k: string) =>
    k in vars ? String(vars[k]) : `{${k}}`,
  );
}

export type Translator = (key: string, vars?: Vars) => string;
/** Pluralizing translator: p(n, "count.article") → "count.article.one|other". */
export type Plural = (n: number, base: string) => string;

interface I18nValue {
  lang: Lang;
  setLang: (lang: Lang) => void;
  t: Translator;
  p: Plural;
  /** Intl locale string for the active language (date formatting). */
  locale: string;
}

const I18nContext = createContext<I18nValue | undefined>(undefined);

const STORAGE_KEY = "tract-language";

function readStoredLang(): Lang {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v === "en" || v === "sv") return v;
  } catch {
    /* localStorage unavailable — fall through to default */
  }
  return DEFAULT_LANG;
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(readStoredLang);

  const setLang = useCallback((next: Lang) => {
    setLangState(next);
    try {
      localStorage.setItem(STORAGE_KEY, next);
    } catch {
      /* persistence best-effort */
    }
  }, []);

  const value = useMemo<I18nValue>(() => {
    const dict = DICTS[lang];
    const t: Translator = (key, vars) =>
      interpolate(dict[key] ?? en[key] ?? key, vars);
    const p: Plural = (n, base) => t(`${base}.${n === 1 ? "one" : "other"}`, { n });
    return { lang, setLang, t, p, locale: LOCALES[lang] };
  }, [lang, setLang]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nValue {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error("useI18n must be used within I18nProvider");
  return ctx;
}
