import LogViewerApp from './LogViewerApp.svelte';
import '../style.css';

const app = new LogViewerApp({ target: document.getElementById('app')! });
export default app;
