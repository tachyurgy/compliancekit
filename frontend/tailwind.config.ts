import type { Config } from 'tailwindcss';

const config: Config = {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        brand: {
          50: '#ecfdf5',
          100: '#d1fae5',
          200: '#a7f3d0',
          300: '#6ee7b7',
          400: '#34d399',
          500: '#10b981',
          600: '#059669', // primary brand (emerald-600)
          700: '#047857',
          800: '#065f46',
          900: '#064e3b',
        },
        caution: {
          50: '#fffbeb',
          100: '#fef3c7',
          400: '#fbbf24',
          500: '#f59e0b',
          600: '#d97706',
        },
        critical: {
          50: '#fef2f2',
          100: '#fee2e2',
          500: '#ef4444',
          600: '#dc2626',
          700: '#b91c1c',
        },
        surface: {
          DEFAULT: '#ffffff',
          muted: '#f9fafb',
          sunken: '#f3f4f6',
        },
        // Inspector Mode palette — deep navy + warm parchment.
        // Used only by the /inspector/* routes; never on the operator UI.
        inspector: {
          ink: '#0d1f3c',         // primary navy: headers, tab labels, body text
          'ink-soft': '#1f2c4f',  // hover / subtle accents
          accent: '#c9a960',      // warm gold for icon highlights and rule lines
          paper: '#f7f5f1',       // page background (warmer than #fff)
          card: '#ffffff',        // content cards on top of the paper
          rule: '#e1ddd5',        // dividers / rules (parchment grey)
          muted: '#5a6478',       // secondary copy (license #, dates)
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
      },
      boxShadow: {
        card: '0 1px 2px 0 rgb(0 0 0 / 0.04), 0 1px 3px 0 rgb(0 0 0 / 0.06)',
        elevated: '0 4px 12px rgb(0 0 0 / 0.06), 0 2px 4px rgb(0 0 0 / 0.04)',
      },
    },
  },
  plugins: [],
};

export default config;
