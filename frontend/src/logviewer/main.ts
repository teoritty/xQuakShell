import { mount } from 'svelte';
import LogViewerApp from './LogViewerApp.svelte';
import '../style.css';

// The log viewer is a second entry point with its own HTML, so it mounts
// separately. See the note in src/main.ts about the removed constructor.
const app = mount(LogViewerApp, { target: document.getElementById('app')! });
export default app;
