import { useEffect, useState } from "react";
import { api } from "./api";
import type { Item } from "./types";

interface Props {
  id: number;
  onClose: () => void;
}

// Reader view: fetches the full item (body + html) and renders the cleaned
// article. Highlight capture is a roadmap item — the endpoint exists, the UI
// for it lands in a later block.
export function Reader({ id, onClose }: Props) {
  const [item, setItem] = useState<Item | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    api
      .getItem(id)
      .then((it) => alive && setItem(it))
      .catch((e) => alive && setError(e instanceof Error ? e.message : "load failed"));
    return () => {
      alive = false;
    };
  }, [id]);

  return (
    <div className="reader">
      <button className="back" onClick={onClose}>
        ← Library
      </button>

      {error && <p className="error" role="alert">{error}</p>}
      {!item && !error && <p className="muted">Loading…</p>}

      {item && (
        <article>
          <h1>{item.title}</h1>
          <p className="byline">
            <a href={item.url} target="_blank" rel="noreferrer">
              {item.siteName || new URL(item.url).hostname}
            </a>
          </p>
          {/* Article HTML comes from go-readability (server-cleaned). Rendering
              it is a deliberate trade-off documented in the README. */}
          {item.html ? (
            <div className="prose" dangerouslySetInnerHTML={{ __html: item.html }} />
          ) : (
            <div className="prose">
              {item.body?.split("\n\n").map((p, i) => <p key={i}>{p}</p>)}
            </div>
          )}
        </article>
      )}
    </div>
  );
}
