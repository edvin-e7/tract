import { useEffect, useRef, useState } from "react";
import { getToken, setToken } from "./api";
import { useI18n } from "./i18n";

// Key button + anchored popover for the TRACT_TOKEN access token. Mounted in
// both the library qbar and the reader bar, so a 401 can be fixed wherever it
// happens. The token is persisted in localStorage (see api.ts) and attached as
// `Authorization: Bearer <token>` on mutating calls only. A dot on the key
// marks "token stored"; the popover never renders the stored value back into
// the field — Clear is the way out.
export function TokenAccess() {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const [stored, setStored] = useState(() => getToken() !== "");
  const wrapRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) inputRef.current?.focus();
  }, [open]);

  // Close on click outside / Escape.
  useEffect(() => {
    if (!open) return;
    function onDown(e: MouseEvent) {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  function save(e: React.FormEvent) {
    e.preventDefault();
    const v = value.trim();
    if (!v) return;
    setToken(v);
    setStored(true);
    setValue("");
    setOpen(false);
  }

  function clear() {
    setToken("");
    setStored(false);
    setValue("");
  }

  return (
    <div className="tokenwrap" ref={wrapRef}>
      <button
        type="button"
        className={`qicon${stored ? " is-set" : ""}`}
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        aria-haspopup="dialog"
        title={stored ? t("token.set") : t("token.aria")}
        aria-label={stored ? t("token.set") : t("token.aria")}
      >
        <svg viewBox="0 0 24 24" aria-hidden>
          <circle cx="8" cy="14" r="4" />
          <path d="m11 11 8-8M15 7l2.5 2.5M18 4l2 2" />
        </svg>
      </button>

      {open && (
        <form className="tokenpop" role="dialog" aria-label={t("token.aria")} onSubmit={save}>
          <label className="tokenpop__label" htmlFor="tract-token-input">
            {t("token.label")}
          </label>
          <input
            id="tract-token-input"
            ref={inputRef}
            type="password"
            autoComplete="off"
            placeholder={stored ? "••••••••••••" : t("token.placeholder")}
            value={value}
            onChange={(e) => setValue(e.target.value)}
          />
          <p className="tokenpop__hint">{t("token.hint")}</p>
          <div className="tokenpop__row">
            {stored && (
              <button type="button" className="btn btn--ghost" onClick={clear}>
                {t("token.clear")}
              </button>
            )}
            <button type="submit" className="btn btn--accent" disabled={!value.trim()}>
              {t("token.save")}
            </button>
          </div>
        </form>
      )}
    </div>
  );
}
