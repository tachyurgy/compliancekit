import type { Config } from 'tailwindcss';

// REQ-012a — operator-app palette aligned to marketing/_shared.css so the
// signup → onboarding → app handoff feels like one product, not two.
//
//   brand     = sage (marketing --sage #0d6f5e) — primary trust colour.
//   critical  = coral (not red) — "needs attention", per brand-voice.md.
//                "Red signals failure; coral signals 'let's fix this.'"
//   caution   = amber — "expiring soon" pills.
//   warm      = marketing's copper (--warm #c5774a) — onboarding accents,
//                friendly highlights, never alarms.
//   gold      = marketing's gold (--gold #caa257) — icon highlights, rules.
//   ink       = marketing's ink/muted scale — body copy, headings.
//   surface   = paper + cream (--bg #fbfaf6) — generous warm backgrounds.
//
// Inspector Mode keeps its own palette (REQ-009).
const config: Config = {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        brand: {
          50: '#e7f1ed',
          100: '#d6e8e1',
          200: '#b9d6cb',
          300: '#8fbeae',
          400: '#4f9784',
          500: '#138772',
          600: '#0d6f5e', // marketing --sage — primary brand
          700: '#0a5648', // marketing --sage-2
          800: '#084237', // marketing --sage-3
          900: '#052f27',
        },
        // Marketing's --sage-tint values surfaced directly for backgrounds.
        sage: {
          DEFAULT: '#0d6f5e',
          tint: '#e7f1ed',
          'tint-2': '#d6e8e1',
        },
        caution: {
          50: '#fdf6e3',
          100: '#f8ebc7',
          400: '#d4a955',
          500: '#b88a36',
          600: '#946d28',
          700: '#6f521d',
        },
        critical: {
          // Coral, not red. Same role (expired / blocking) but warmer.
          50: '#fdece4',
          100: '#fbd9c8',
          200: '#f6b89c',
          400: '#ea8c69',
          500: '#d96a43',
          600: '#c5552d', // primary coral — pairs with sage
          700: '#a1421e',
          800: '#7d3217',
        },
        warm: {
          DEFAULT: '#c5774a',
          50: '#fbeee2',
          100: '#f6dcc4',
          500: '#c5774a', // marketing --warm
          600: '#a85d35', // marketing --warm-2
        },
        gold: {
          DEFAULT: '#caa257',
          tint: '#f6efdc',
          500: '#caa257', // marketing --gold
        },
        ink: {
          DEFAULT: '#0c1722',
          900: '#0c1722', // marketing --ink
          800: '#243345', // marketing --ink-2
          700: '#3a4a5d', // marketing --ink-3
          500: '#5a6a7a', // marketing --muted
          400: '#8194a4', // marketing --muted-2
        },
        line: {
          DEFAULT: '#e6ebf0',
          soft: '#eef2f5',
        },
        surface: {
          DEFAULT: '#ffffff',          // --paper
          muted: '#fbfaf6',            // --bg (cream — body background)
          sunken: '#f5f1e6',           // --bg-2 (warmer cream — sunken panels)
        },
        // Inspector Mode palette — deep navy + warm parchment.
        // Used only by the /inspector/* routes; never on the operator UI.
        inspector: {
          ink: '#0d1f3c',
          'ink-soft': '#1f2c4f',
          accent: '#c9a960',
          paper: '#f7f5f1',
          card: '#ffffff',
          rule: '#e1ddd5',
          muted: '#5a6478',
        },
      },
      fontFamily: {
        sans: [
          'Inter',
          'system-ui',
          '-apple-system',
          'Segoe UI',
          'Roboto',
          'sans-serif',
        ],
        display: [
          'Fraunces',
          'ui-serif',
          'Georgia',
          'serif',
        ],
      },
      borderRadius: {
        // Match marketing radii so cards line up visually with the marketing site.
        lb: '14px',
        'lb-sm': '10px',
        'lb-lg': '20px',
      },
      boxShadow: {
        card: '0 1px 2px rgba(12,23,34,.04), 0 1px 1px rgba(12,23,34,.03)',
        elevated: '0 4px 16px rgba(12,23,34,.06), 0 1px 3px rgba(12,23,34,.04)',
        hover: '0 24px 56px -16px rgba(12,23,34,.18), 0 8px 16px -8px rgba(12,23,34,.06)',
      },
    },
  },
  plugins: [],
};

export default config;
