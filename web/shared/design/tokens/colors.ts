export const colors = {
  bg: {
    canvas: 'var(--color-bg-canvas)',
    surface: 'var(--color-bg-surface)',
    surfaceMuted: 'var(--color-bg-surfaceMuted)',
    elevated: 'var(--color-bg-elevated)',
    overlay: 'var(--color-bg-overlay)',
  },
  border: {
    default: 'var(--color-border-default)',
    strong: 'var(--color-border-strong)',
    subtle: 'var(--color-border-subtle)',
  },
  text: {
    primary: 'var(--color-text-primary)',
    secondary: 'var(--color-text-secondary)',
    muted: 'var(--color-text-muted)',
    inverse: 'var(--color-text-inverse)',
    link: 'var(--color-text-link)',
  },
  accent: {
    primary: 'var(--color-accent-primary)',
    hover: 'var(--color-accent-primaryHover)',
    active: 'var(--color-accent-primaryActive)',
    muted: 'var(--color-accent-primaryMuted)',
  },
  state: {
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
  },
  brand: {
    orange: 'var(--color-brand-orange)',
    orangeHover: 'var(--color-brand-orangeHover)',
  },
} as const
