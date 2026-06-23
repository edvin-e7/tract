export interface Item {
  id: number;
  url: string;
  title: string;
  excerpt: string;
  siteName: string;
  createdAt: string;
  highlightCount: number;
  // present only on the single-item (reader) fetch
  body?: string;
  html?: string;
  highlights?: Highlight[];
}

export interface Highlight {
  id: number;
  itemId: number;
  text: string;
  createdAt: string;
}
