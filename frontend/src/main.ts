import '@xterm/xterm/css/xterm.css'
import './style.css'
import { mount } from 'svelte'
import App from './App.svelte'
import { initLocale } from './i18n/apply'

// Svelte 5 removed the `new Component({ target })` constructor. mount() is its
// replacement and the only part of this codebase the compiler change reached -
// everything else runs unaltered in legacy mode.
// The language is loaded before the app mounts, not after. Mounting first and translating on
// arrival would draw one English frame, and the screen that frame shows is the master password
// prompt - the one screen a user who does not read English most needs in their own language.
await initLocale()

const app = mount(App, {
  target: document.getElementById('app')!
})

export default app
