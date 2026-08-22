const KEY = 'theme'

function apply(dark) {
  document.documentElement.classList.toggle('dark', dark)
}

export const theme = $state({
  dark: document.documentElement.classList.contains('dark')
})

export function toggleTheme() {
  theme.dark = !theme.dark
  localStorage.setItem(KEY, theme.dark ? 'dark' : 'light')
  apply(theme.dark)
}

const mq = matchMedia('(prefers-color-scheme: dark)')
mq.addEventListener('change', (e) => {
  if (localStorage.getItem(KEY)) return
  theme.dark = e.matches
  apply(e.matches)
})
