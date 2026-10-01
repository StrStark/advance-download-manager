const box = document.getElementById('enabled')
const status = document.getElementById('status')

chrome.storage.local.get({ enabled: true }).then((v) => (box.checked = v.enabled))
box.addEventListener('change', () => chrome.storage.local.set({ enabled: box.checked }))

chrome.runtime.sendMessage('ping').then((r) => {
  if (r?.ok) {
    status.innerHTML = `<span class="ok">● Connected</span> to ADM ${r.version || ''}`
  } else {
    status.innerHTML =
      '<span class="bad">● ADM not found.</span> Install and open ADM once, then restart the browser. ' +
      '<a href="https://github.com/StrStark/advance-download-manager/releases/latest" target="_blank">Get ADM</a>'
  }
})
