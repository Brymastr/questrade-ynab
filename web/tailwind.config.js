/** @type {import('tailwindcss').Config} */
const token = (name) => `rgb(var(--color-${name}) / <alpha-value>)`

export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: 'media',
  theme: {
    extend: {
      colors: {
        bg: token('bg'),
        surface: token('surface'),
        'surface-2': token('surface-2'),
        // Named `line`, not `border`, so it can't shadow the `border` utility.
        line: token('line'),
        fg: token('fg'),
        'fg-muted': token('fg-muted'),
        'fg-faint': token('fg-faint'),
        primary: token('primary'),
        'primary-fg': token('primary-fg'),
        accent: token('accent'),
        pos: token('pos'),
        neg: token('neg'),
        warn: token('warn'),
      },
      borderColor: {
        DEFAULT: token('line'),
      },
      fontFamily: {
        sans: [
          '"Inter Variable"',
          'Inter',
          'system-ui',
          '-apple-system',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'sans-serif',
        ],
      },
    },
  },
  plugins: [],
}
