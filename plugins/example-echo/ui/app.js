const PLUGIN_ID = 'com.xquakshell.example-echo';

const PANEL_ID = 'echo.panel';

const log = document.getElementById('log');



const params = new URLSearchParams(window.location.search);

const HOST_ORIGIN = params.get('hostOrigin') || window.location.origin;



/** @type {MessagePort | null} */

let hostPort = null;



function handleHostPayload(data) {

  if (!data || data.source !== 'xquakshell-host') return;

  if (data.pluginId !== PLUGIN_ID || data.panelId !== PANEL_ID) return;

  log.textContent = 'Reply: ' + JSON.stringify(data.message);

}



function sendToHost(message) {

  const payload = {

    source: 'xquakshell-plugin-view',

    pluginId: PLUGIN_ID,

    panelId: PANEL_ID,

    message,

  };

  if (hostPort) {

    hostPort.postMessage(payload);

    return;

  }

  parent.postMessage(payload, HOST_ORIGIN);

}



document.getElementById('ping').addEventListener('click', () => {

  sendToHost({ action: 'ping' });

});



window.addEventListener('message', (event) => {

  if (event.origin !== HOST_ORIGIN && event.origin !== 'null') return;

  const data = event.data;

  if (

    data &&

    data.source === 'xquakshell-host-init' &&

    data.pluginId === PLUGIN_ID &&

    data.panelId === PANEL_ID &&

    event.ports &&

    event.ports[0]

  ) {

    hostPort = event.ports[0];

    hostPort.onmessage = (portEvent) => handleHostPayload(portEvent.data);

    return;

  }

  handleHostPayload(data);

});

