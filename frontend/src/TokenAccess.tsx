import { useEffect, useRef, useState } from "react";
import { getServer, getToken, setServer, setToken } from "./api";
import { useI18n } from "./i18n";

// Key button + anchored popover for the connection settings: the server
// address (native shells / remote self-host — empty means same-origin) and the
// TRACT_TOKEN access token. Mounted in both the library qbar and the reader
// bar, so a 401 or an unconfigured shell can be fixed wherever it happens.
// Both persist in localStorage (see api.ts). A dot on the key marks "token
// stored"; the server address is not a secret and renders back into its field,
// but the popover never renders the stored token — Clear is the way out.
export function TokenAccess() {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const [server, setServerValue] = useState(() => getServer());
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
    const serverChanged = server.trim().replace(/\/+$/, "") !== getServer();
    setServer(server);
    const v = value.trim();
    const tokenChanged = v !== "" && v !== getToken();
    if (v) {
      setToken(v);
      setStored(true);
      setValue("");
    }
    setOpen(false);
    // A server change redirects every API call to a new origin, and — since the
    // token now rides on reads too — a token change changes what those calls are
    // allowed to SEE. Both need the same refetch.
    //
    // Measured on the first-run path of a TRACT_PRIVATE=1 server before this
    // line existed: open the app, get "this server requires an access token",
    // paste the token, press Save — and the error stays, because nothing re-ran
    // the load that failed. The token was stored correctly and the app looked
    // exactly as broken as it had a second earlier. The fix is only visible on
    // the one path where a user has no idea anything is wrong yet.
    if (serverChanged || tokenChanged) window.location.reload();
  }

  function clear() {
    const had = getToken() !== "";
    setToken("");
    setStored(false);
    setValue("");
    // Same reason, other direction: on a private server, dropping the token
    // must drop back to the locked state rather than leaving the last
    // authorised render on screen looking like open access.
    if (had) window.location.reload();
  }

  return (
    <div className="tokenwrap" ref={wrapRef}>
      <button
        type="button"
        className={`qicon${stored ? " is-set" : ""}`}
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        aria-haspopup="dialog"
        title={stored ? t("token.set") : t("conn.aria")}
        aria-label={stored ? t("token.set") : t("conn.aria")}
      >
        <svg viewBox="0 0 24 24" aria-hidden>
          <circle cx="8" cy="14" r="4" />
          <path d="m11 11 8-8M15 7l2.5 2.5M18 4l2 2" />
        </svg>
      </button>

      {open && (
        <form className="tokenpop" role="dialog" aria-label={t("conn.aria")} onSubmit={save}>
          <label className="tokenpop__label" htmlFor="tract-server-input">
            {t("server.label")}
          </label>
          <input
            id="tract-server-input"
            ref={inputRef}
            type="url"
            autoComplete="off"
            placeholder={t("server.placeholder")}
            value={server}
            onChange={(e) => setServerValue(e.target.value)}
          />
          <p className="tokenpop__hint">{t("server.hint")}</p>
          <label className="tokenpop__label" htmlFor="tract-token-input">
            {t("token.label")}
          </label>
          <input
            id="tract-token-input"
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
            <button type="submit" className="btn btn--accent">
              {t("token.save")}
            </button>
          </div>
        </form>
      )}
    </div>
  );
}
