import type { Config } from 'tailwindcss'

const config: Config = {
  content: [
    './app.vue',
    './layouts/**/*.{vue,js,ts}',
    './pages/**/*.vue',
    './widgets/**/*.vue',
    './features/**/*.vue',
    './entities/**/*.vue',
    './shared/**/*.vue',
    './components/**/*.vue',
  ],
  theme: {
    extend: {
      colors: {
        // Background
        canvas: 'var(--color-bg-canvas)',
        surface: 'var(--color-bg-surface)',
        surfaceMuted: 'var(--color-bg-surfaceMuted)',
        elevated: 'var(--color-bg-elevated)',
        overlay: 'var(--color-bg-overlay)',
        // Border
        borderDefault: 'var(--color-border-default)',
        borderStrong: 'var(--color-border-strong)',
        borderSubtle: 'var(--color-border-subtle)',
        // Text
        textPrimary: 'var(--color-text-primary)',
        textSecondary: 'var(--color-text-secondary)',
        textMuted: 'var(--color-text-muted)',
        textInverse: 'var(--color-text-inverse)',
        textLink: 'var(--color-text-link)',
        // Accent
        accent: 'var(--color-accent-primary)',
        accentHover: 'var(--color-accent-primaryHover)',
        accentActive: 'var(--color-accent-primaryActive)',
        accentMuted: 'var(--color-accent-primaryMuted)',
        // State
        success: 'var(--color-state-success)',
        successMuted: 'var(--color-state-successMuted)',
        warning: 'var(--color-state-warning)',
        warningMuted: 'var(--color-state-warningMuted)',
        error: 'var(--color-state-error)',
        errorMuted: 'var(--color-state-errorMuted)',
        info: 'var(--color-state-info)',
        infoMuted: 'var(--color-state-infoMuted)',
        neutral: 'var(--color-state-neutral)',
        neutralMuted: 'var(--color-state-neutralMuted)',
        // Brand
        brandOrange: 'var(--color-brand-orange)',
        brandOrangeHover: 'var(--color-brand-orangeHover)',
      },
      borderRadius: {
        none: 'var(--radius-none)',
        sm: 'var(--radius-sm)',
        md: 'var(--radius-md)',
        lg: 'var(--radius-lg)',
        xl: 'var(--radius-xl)',
        '2xl': 'var(--radius-2xl)',
        full: 'var(--radius-full)',
      },
      spacing: {
        0: 'var(--space-0)',
        1: 'var(--space-1)',
        2: 'var(--space-2)',
        3: 'var(--space-3)',
        4: 'var(--space-4)',
        5: 'var(--space-5)',
        6: 'var(--space-6)',
        8: 'var(--space-8)',
        10: 'var(--space-10)',
        12: 'var(--space-12)',
        16: 'var(--space-16)',
      },
      fontFamily: {
        sans: ['var(--font-sans)'],
        mono: ['var(--font-mono)'],
      },
      fontSize: {
        xs: 'var(--text-xs)',
        sm: 'var(--text-sm)',
        base: 'var(--text-base)',
        lg: 'var(--text-lg)',
        xl: 'var(--text-xl)',
        '2xl': 'var(--text-2xl)',
        '3xl': 'var(--text-3xl)',
      },
      boxShadow: {
        sm: 'var(--shadow-sm)',
        md: 'var(--shadow-md)',
        lg: 'var(--shadow-lg)',
        xl: 'var(--shadow-xl)',
      },
      transitionDuration: {
        fast: '100ms',
        normal: '150ms',
        slow: '300ms',
      },
      zIndex: {
        base: 'var(--z-base)',
        dropdown: 'var(--z-dropdown)',
        sticky: 'var(--z-sticky)',
        overlay: 'var(--z-overlay)',
        modal: 'var(--z-modal)',
        toast: 'var(--z-toast)',
      },
    },
  },
  plugins: [],
}

export default config
