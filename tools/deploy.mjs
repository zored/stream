import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { basename } from 'node:path'
import { connect } from './trpc.mjs'

const CHUNK = 64 * 1024

const args = process.argv.slice(2)
const varsFlag = args.indexOf('--vars')
const variables = varsFlag === -1 ? {} : JSON.parse(readFileSync(args.splice(varsFlag, 2)[1], 'utf8'))

const targets = args.map((arg) => {
	const [file, page] = arg.split(':')
	if (!file || !page) throw new Error(`expected <file>:<page>, got "${arg}"`)
	return { file, page: Number(page) }
})
if (targets.length === 0) throw new Error('usage: node tools/deploy.mjs [--vars <file>] <file>:<page> ...')

const { mutate, close } = await connect()

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

async function ensureVariable(name, defaultVal) {
	const error = await mutate('customVariables.create', { name, defaultVal })
	if (error && error !== `Variable "${name}" already exists`) throw new Error(`custom variable ${name}: ${error}`)
}

try {
	for (const [name, defaultVal] of Object.entries(variables)) await ensureVariable(name, defaultVal)
	for (const target of targets) await deploy(target)
} finally {
	close()
}
