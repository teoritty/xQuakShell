import '@xterm/xterm/css/xterm.css'
import './style.css'
import { mount } from 'svelte'
import App from './App.svelte'

// Svelte 5 removed the `new Component({ target })` constructor. mount() is its
// replacement and the only part of this codebase the compiler change reached -
// everything else runs unaltered in legacy mode.
const app = mount(App, {
  target: document.getElementById('app')!
})

export default app
