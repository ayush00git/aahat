// Inline SVG icons (24×24, stroked). Decorative: always aria-hidden; the text
// next to each icon carries the meaning.

import type { JSX } from 'preact';

type P = { size?: number; class?: string };

function Svg({ size = 24, class: cls, children, fill }: P & { children: JSX.Element | JSX.Element[]; fill?: boolean }) {
  return (
    <svg
      class={cls ? `icon ${cls}` : 'icon'}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill={fill ? 'currentColor' : 'none'}
      stroke={fill ? 'none' : 'currentColor'}
      stroke-width="2.2"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {children}
    </svg>
  );
}

/** Rising water: in the flood path. */
export const IconFlood = (p: P) => (
  <Svg {...p}>
    <path d="M12 3v8m-3.5-3.5L12 11l3.5-3.5" />
    <path d="M2 16c1.7 0 1.7-1.5 3.3-1.5S7 16 8.7 16s1.6-1.5 3.3-1.5 1.7 1.5 3.3 1.5 1.7-1.5 3.4-1.5S20.3 16 22 16" />
    <path d="M2 20.5c1.7 0 1.7-1.5 3.3-1.5S7 20.5 8.7 20.5s1.6-1.5 3.3-1.5 1.7 1.5 3.3 1.5 1.7-1.5 3.4-1.5 1.6 1.5 3.3 1.5" />
  </Svg>
);

/** Warning triangle: at risk / alert. */
export const IconWarn = (p: P) => (
  <Svg {...p}>
    <path d="M10.3 3.9 2.4 17.6A2 2 0 0 0 4.1 20.6h15.8a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z" />
    <path d="M12 9.5v4.5M12 17.2v.1" />
  </Svg>
);

/** A house beside moving water: a flood passing nearby. */
export const IconNearby = (p: P) => (
  <Svg {...p}>
    <path d="M2.5 11 7 7l4.5 4v8.5h-9Z" />
    <path d="M14 8c1.3 0 1.3-1 2.7-1s1.3 1 2.6 1 1.4-1 2.7-1M14 13c1.3 0 1.3-1 2.7-1s1.3 1 2.6 1 1.4-1 2.7-1M14 18c1.3 0 1.3-1 2.7-1s1.3 1 2.6 1 1.4-1 2.7-1" />
  </Svg>
);

export const IconCheck = (p: P) => (
  <Svg {...p}>
    <path d="m5 12.5 4.5 4.5L19 7.5" />
  </Svg>
);

/** Dashed circle with a question: not covered. */
export const IconUnknown = (p: P) => (
  <Svg {...p}>
    <path d="M12 2.8a9.2 9.2 0 1 1 0 18.4 9.2 9.2 0 0 1 0-18.4Z" stroke-dasharray="2.6 2.6" />
    <path d="M9.6 9.4a2.5 2.5 0 1 1 3.4 2.3c-.6.3-1 .8-1 1.5v.4M12 16.6v.1" />
  </Svg>
);

export const IconSearch = (p: P) => (
  <Svg {...p}>
    <path d="M10.5 3.5a7 7 0 1 1 0 14 7 7 0 0 1 0-14ZM20.5 20.5l-5-5" />
  </Svg>
);

export const IconClock = (p: P) => (
  <Svg {...p}>
    <path d="M12 3a9 9 0 1 1 0 18 9 9 0 0 1 0-18Z" />
    <path d="M12 7.5V12l3 2" />
  </Svg>
);

/** Winding river: distance along the river. */
export const IconRiver = (p: P) => (
  <Svg {...p}>
    <path d="M6 2.5c4 2 4 5 0 7.5s-4 6 1 8.5 5 3 5 3" />
    <path d="M14 2.5c4 2 4 5 0 7.5s-3 5 3 8 3 3 3 3" />
  </Svg>
);

export const IconPin = (p: P) => (
  <Svg {...p}>
    <path d="M12 21.5s-7-6.2-7-11.5a7 7 0 1 1 14 0c0 5.3-7 11.5-7 11.5Z" />
    <path d="M12 7.5a2.5 2.5 0 1 1 0 5 2.5 2.5 0 0 1 0-5Z" />
  </Svg>
);

export const IconMap = (p: P) => (
  <Svg {...p}>
    <path d="M9 4 3 6.5v13.5L9 17.5l6 2.5 6-2.5V4l-6 2.5L9 4Z" />
    <path d="M9 4v13.5M15 6.5V20" />
  </Svg>
);

export const IconBell = (p: P) => (
  <Svg {...p}>
    <path d="M6 16.5V11a6 6 0 0 1 12 0v5.5l1.5 2h-15l1.5-2Z" />
    <path d="M10 21h4" />
  </Svg>
);

export const IconSms = (p: P) => (
  <Svg {...p}>
    <path d="M4 4.5h16a1 1 0 0 1 1 1V16a1 1 0 0 1-1 1H9l-5 4v-4a1 1 0 0 1-1-1V5.5a1 1 0 0 1 1-1Z" />
    <path d="M8 9h8M8 12.5h5" />
  </Svg>
);

export const IconPhone = (p: P) => (
  <Svg {...p}>
    <path d="M5 3h3.5l1.8 4.5-2.3 1.4a11 11 0 0 0 7.1 7.1l1.4-2.3L21 15.5V19a2 2 0 0 1-2 2A17 17 0 0 1 3 5a2 2 0 0 1 2-2Z" />
  </Svg>
);

export const IconChevron = (p: P) => (
  <Svg {...p}>
    <path d="m9 5 7 7-7 7" />
  </Svg>
);

export const IconBack = (p: P) => (
  <Svg {...p}>
    <path d="m15 5-7 7 7 7" />
  </Svg>
);

/** Arrow up a slope: go to high ground. */
export const IconUphill = (p: P) => (
  <Svg {...p}>
    <path d="M2.5 20.5h19L14 9l-3.5 5-2-2.5-6 9Z" />
    <path d="M17 7V2.5m-2.3 2.2L17 2.5l2.3 2.2" />
  </Svg>
);

export const IconPlay = (p: P) => (
  <Svg {...p} fill>
    <path d="M7 4.2v15.6a1 1 0 0 0 1.5.9l12.3-7.8a1 1 0 0 0 0-1.8L8.5 3.3A1 1 0 0 0 7 4.2Z" />
  </Svg>
);

export const IconPause = (p: P) => (
  <Svg {...p} fill>
    <path d="M6 4h4v16H6zM14 4h4v16h-4z" />
  </Svg>
);

/** Speech bubble with a phone handset: WhatsApp-style share. Drawn here, not a brand asset. */
export const IconWhatsApp = ({ size = 24, class: cls }: P) => (
  <svg
    class={cls ? `icon ${cls}` : 'icon'}
    width={size}
    height={size}
    viewBox="0 0 24 24"
    aria-hidden="true"
    focusable="false"
  >
    <path
      d="M12 3a8.5 8.5 0 1 1-4.3 15.8L3.5 20l1.2-4A8.5 8.5 0 0 1 12 3Z"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linejoin="round"
    />
    <path
      fill="currentColor"
      d="M9.3 7.6c.3-.1.6 0 .7.3l.8 1.8c.1.3 0 .6-.2.8l-.6.7c.6 1.2 1.6 2.2 2.8 2.8l.7-.6c.2-.2.5-.3.8-.2l1.8.8c.3.1.4.4.3.7l-.3 1c-.2.6-.7.9-1.3.9A7 7 0 0 1 8 9.7c0-.6.3-1.1.9-1.3Z"
    />
  </svg>
);
