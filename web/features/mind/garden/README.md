# Calendar garden

`vine.js` is the numerical topology solver and `renderer.js` is the procedural
SVG renderer extracted from the owner-approved 2026-10-19 Butler calendar.
They retain its branch layout, growth/morph transitions and foliage. The renderer
accepts an injected host, stage and motion owner; it has no application singleton,
network calls or demonstration events. Static SVG templates contain only authored
geometry. Vue renders task text; it is never interpolated into SVG HTML.

`CalendarGarden.vue` supplies real calendar items, local dates and motion settings.
One instance owns observers, listeners, animation frames and bloom/highlight timers.
Unmount calls `destroy()` to cancel them and remove its DOM. Hidden/reduced-motion
states finish interpolation. Bloom timing is visual, not proof of verification.

`calendar.css` contains applicable approved styles scoped to `.calendar-garden`.
Application/modal styles are in `assets/css/mind.css`. Responsive/calendar smoke
coverage is in `mind.spec.ts`.
