import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { basename } from 'node:path'

const url = process.env.COMPANION_TRPC ?? 'ws://127.0.0.1:8000/trpc'
const CHUNK = 64 * 1024

const targets = process.argv.slice(2).map((arg) => {
	const [file, page] = arg.split(':')
	if (!file || !page) throw new Error(`expected <file>:<page>, got "${arg}"`)
	return { file, page: Number(page) }
})
if (targets.length === 0) throw new Error('usage: node tools/deploy.mjs <file>:<page> ...')

const ws = new WebSocket(url)
const pending = new Map()
let nextId = 1

ws.addEventListener('message', (event) => {
	const msg = JSON.parse(event.data)
	const call = pending.get(msg.id)
	if (!call) return
	pending.delete(msg.id)
	if (msg.error) call.reject(new Error(`${call.path}: ${msg.error.message}`))
	else call.resolve(msg.result?.data)
})
ws.addEventListener('close', () => {
	for (const call of pending.values()) call.reject(new Error(`${call.path}: connection closed`))
})

function mutate(path, input) {
	const id = nextId++
	return new Promise((resolve, reject) => {
		pending.set(id, { path, resolve, reject })
		ws.send(JSON.stringify({ id, method: 'mutation', params: { path, input } }))
	})
}

async function deploy({ file, page }) {
	const data = readFileSync(file)
	const sessionId = await mutate('importExport.prepareImport.start', { name: basename(file), size: data.length })
	for (let offset = 0; offset < data.length; offset += CHUNK) {
		const chunk = data.subarray(offset, offset + CHUNK).toString('base64')
		await mutate('importExport.prepareImport.uploadChunk', { sessionId, offset, data: chunk })
	}
	const checksum = createHash('sha1').update(data).digest('hex')
	const [error] = await mutate('importExport.prepareImport.complete', { sessionId, expectedChecksum: checksum, userData: null })
	if (error) throw new Error(`${file}: ${error}`)
	await mutate('importExport.importSinglePage', { targetPage: page, sourcePage: 1, connectionIdRemapping: {} })
	await mutate('importExport.abort', undefined)
	console.log(`${file} → page ${page}`)
}

await new Promise((resolve, reject) => {
	ws.addEventListener('open', resolve, { once: true })
	ws.addEventListener('error', () => reject(new Error(`can't connect to ${url}`)), { once: true })
})
try {
	for (const target of targets) await deploy(target)
} finally {
	ws.close()
}
