// Catches downloads the browser starts and hands them to ADM through native
// messaging. If ADM isn't installed or doesn't answer, the browser downloads
// the file itself as usual.
const HOST = 'io.github.adm'
const RELEASES = 'https://github.com/StrStark/advance-download-manager/releases/latest'

let enabled = true
const passThrough = new Map() // url → expiry: re-downloads we started ourselves

chrome.storage.local.get({ enabled: true }).then((v) => (enabled = v.enabled))
chrome.storage.onChanged.addListener((c) => {
  if (c.enabled) enabled = c.enabled.newValue
})

chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.create({ id: 'adm-link', title: 'Download with ADM', contexts: ['link', 'video', 'audio', 'image'] })
})

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  const url = info.linkUrl || info.srcUrl
  if (!url || !/^https?:/i.test(url)) return
  const ok = await sendToADM({ url, referrer: info.pageUrl || tab?.url || '' })
  if (!ok) chrome.tabs.create({ url: RELEASES })
})

chrome.downloads.onCreated.addListener(async (item) => {
  const url = item.finalUrl || item.url
  if (!enabled || !/^https?:/i.test(url)) return
  const until = passThrough.get(url)
  if (until && until > Date.now()) {
    passThrough.delete(url)
    return
  }
  if (item.state && item.state !== 'in_progress') return

  // Stop the browser's own download, then hand the link to ADM.
  try {
    await chrome.downloads.cancel(item.id)
  } catch {
    /* already finished or gone */
  }
  const ok = await sendToADM({
    url,
    filename: baseName(item.filename),
    referrer: item.referrer || '',
    size: item.fileSize > 0 ? item.fileSize : item.totalBytes > 0 ? item.totalBytes : 0,
    mime: item.mime || '',
  })
  if (ok) {
    chrome.downloads.erase({ id: item.id }).catch(() => {})
    flashBadge('✓', '#10b981')
  } else {
    // ADM isn't available: let the browser download it after all.
    passThrough.set(url, Date.now() + 30000)
    chrome.downloads.download({ url }).catch(() => {})
    flashBadge('!', '#f59e0b')
  }
})

async function sendToADM(d) {
  d.cookies = await cookieHeader(d.url)
  d.userAgent = navigator.userAgent
  try {
    const r = await chrome.runtime.sendNativeMessage(HOST, { type: 'download', ...d })
    return !!(r && r.ok)
  } catch (e) {
    console.warn('ADM not reachable:', e)
    return false
  }
}

// The download may need the site's session; pass its cookies along.
async function cookieHeader(url) {
  try {
    const cs = await chrome.cookies.getAll({ url })
    return cs.map((c) => `${c.name}=${c.value}`).join('; ')
  } catch {
    return ''
  }
}

function baseName(p) {
  return (p || '').split(/[\\/]/).pop() || ''
}

function flashBadge(text, color) {
  chrome.action.setBadgeBackgroundColor({ color })
  chrome.action.setBadgeText({ text })
  setTimeout(() => chrome.action.setBadgeText({ text: '' }), 2500)
}

// The popup asks whether ADM answers.
chrome.runtime.onMessage.addListener((msg, _sender, respond) => {
  if (msg === 'ping') {
    chrome.runtime
      .sendNativeMessage(HOST, { type: 'ping' })
      .then((r) => respond({ ok: !!r?.ok, version: r?.version }))
      .catch((e) => respond({ ok: false, error: String(e?.message || e) }))
    return true
  }
})
