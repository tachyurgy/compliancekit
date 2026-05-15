// REQ-024 Shayna avatar — inline SVG so first paint has no extra request.
// Loosely-illustrated Florida-director persona: warm skin, dark hair tied
// back, yellow polo. Not photoreal — we lean into "friendly cartoon" so
// the operator doesn't expect a chatbot.

interface Props {
  size?: number;
  className?: string;
}

export default function ShaynaAvatar({ size = 64, className }: Props): JSX.Element {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 64 64"
      role="img"
      aria-label="Shayna, your guide"
      className={className}
    >
      {/* Background circle */}
      <circle cx="32" cy="32" r="32" fill="#FFEFD0" />
      {/* Hair back */}
      <path
        d="M14 33c0-12 8-21 18-21s18 9 18 21v8H14v-8z"
        fill="#3F2A1A"
      />
      {/* Neck */}
      <rect x="27" y="42" width="10" height="8" rx="2" fill="#D9A079" />
      {/* Shirt — warm yellow polo */}
      <path
        d="M16 64v-8c0-5 4-9 9-9h14c5 0 9 4 9 9v8H16z"
        fill="#F6C56A"
      />
      {/* Collar V */}
      <path d="M28 47l4 5 4-5-1-2-3 2-3-2-1 2z" fill="#E5A12B" />
      {/* Face */}
      <ellipse cx="32" cy="34" rx="11" ry="13" fill="#E8B894" />
      {/* Hair front fringe */}
      <path
        d="M21 28c2-7 7-11 11-11s9 4 11 11c-3-2-7-3-11-3s-8 1-11 3z"
        fill="#3F2A1A"
      />
      {/* Side hair sweep */}
      <path d="M20 31c1 4 2 7 4 9-2-1-3-3-4-9z" fill="#2C1D11" />
      {/* Eyebrows */}
      <path d="M25 31 q2-1 4 0" stroke="#2C1D11" strokeWidth="1.3" fill="none" strokeLinecap="round" />
      <path d="M35 31 q2-1 4 0" stroke="#2C1D11" strokeWidth="1.3" fill="none" strokeLinecap="round" />
      {/* Eyes */}
      <circle cx="27" cy="34" r="1.3" fill="#1B1209" />
      <circle cx="37" cy="34" r="1.3" fill="#1B1209" />
      {/* Cheek blush */}
      <circle cx="24" cy="38" r="1.6" fill="#F2A989" opacity="0.55" />
      <circle cx="40" cy="38" r="1.6" fill="#F2A989" opacity="0.55" />
      {/* Smile */}
      <path d="M28 40 q4 3 8 0" stroke="#7A2E1B" strokeWidth="1.4" fill="none" strokeLinecap="round" />
      {/* Earring dots */}
      <circle cx="20.5" cy="36" r="1" fill="#E5A12B" />
      <circle cx="43.5" cy="36" r="1" fill="#E5A12B" />
    </svg>
  );
}
