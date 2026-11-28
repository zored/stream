import { execFile } from 'node:child_process'
import { mkdir, rename, writeFile } from 'node:fs/promises'
import { dirname } from 'node:path'
import { promisify } from 'node:util'
import { connect } from '../trpc.mjs'

const [dialctl, output] = process.argv.slice(2)
if (!dialctl || !output) throw new Error('Usage: node battery-levels.mjs <dialctl.exe> <battery-levels.json>')

const api = 'http://127.0.0.1:8000'
const variable = 'battery_d100h_svg'
const expression = { isExpression: true, value: '$(custom:battery_d100h_svg)' }
const run = promisify(execFile)

const sources = {
	d100h: async () => {
		const { stdout } = await run(dialctl, ['-gatt', 'battery'], { windowsHide: true, timeout: 35000 })
		return JSON.parse(stdout).percent
	},
}

async function readLevels() {
	const levels = {}
	const failures = []
	await Promise.all(Object.entries(sources).map(async ([id, read]) => {
		try {
			const percent = await read()
			if (!Number.isInteger(percent) || percent < 0 || percent > 100) {
				throw new Error('Invalid battery level: ' + percent)
			}
			levels[id] = percent
		} catch (error) {
			failures.push(new Error(id + ': ' + error.message, { cause: error }))
		}
	}))
	await mkdir(dirname(output), { recursive: true })
	const pending = output + '.' + process.pid + '.tmp'
	await writeFile(pending, JSON.stringify(levels) + '\n')
	await rename(pending, output)
	return { levels, failures }
}

async function publish(percent) {
	const client = await connect()
	try {
		const pages = await client.first('pages.watch')
		const media = pages.pages[pages.order[0]]
		const controlId = media?.controls?.[0]?.[4]
		if (media?.name !== 'MEDIA' || !controlId) throw new Error('Companion MEDIA tile 1/0/4 is missing')
		const { config } = await client.first('controls.watchControl', { controlId })
		const image = config?.style?.layers?.find(layer => layer.id === 'layout')?.base64Image
		if (!image?.isExpression || !['$(custom:input_svg)', expression.value].includes(image.value)) {
			throw new Error('Companion MEDIA tile 1/0/4 does not have the expected layout layer')
		}
		const error = await client.mutate('customVariables.create', { name: variable, defaultVal: '' })
		if (error && error !== 'Variable "' + variable + '" already exists') throw new Error(error)
		const text = percent ?? '&#8212;'
		const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="88" height="88"><text x="44" y="56" fill="white" font-size="34" font-family="JetBrains Mono, monospace" text-anchor="middle">' + text + '</text></svg>'
		const data = 'data:image/svg+xml;base64,' + Buffer.from(svg).toString('base64')
		const response = await fetch(api + '/api/custom-variable/' + variable + '/value', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(data),
			signal: AbortSignal.timeout(5000),
		})
		if (!response.ok) throw new Error('Companion rejected battery tile update: HTTP ' + response.status)
		if (image.value !== expression.value) {
			const updated = await client.mutate('controls.styles.updateOption', {
				controlId, elementId: 'layout', key: 'base64Image', value: expression,
			})
			if (!updated) throw new Error('Companion rejected battery tile style')
		}
	} finally {
		client.close()
	}
}

const { levels, failures } = await readLevels()
try {
	await publish(levels.d100h)
} catch (error) {
	failures.push(error)
}
if (failures.length) throw new AggregateError(failures, 'Battery refresh failed')
