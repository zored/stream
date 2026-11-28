import { execFile } from 'node:child_process'
import { mkdir, rename, writeFile } from 'node:fs/promises'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'
import { connect } from '../trpc.mjs'

const [dialctl, output] = process.argv.slice(2)
if (!dialctl || !output) throw new Error('Usage: node battery-levels.mjs <dialctl.exe> <battery-levels.json>')

const api = 'http://127.0.0.1:8000'
const variable = 'battery_d100h_svg'
const expression = { isExpression: true, value: '$(custom:battery_d100h_svg)' }
const run = promisify(execFile)

const tiles = { d100h: 'dial', xbox: 'xbox', sense_left: 'vr2 l', sense_right: 'vr2 r', steam: 'steam' }
const gamepads = fileURLToPath(new URL('./gamepad-battery.ps1', import.meta.url))
const steamSource = fileURLToPath(new URL('./battery-levels.steam.cs', import.meta.url))

async function readDial() {
	try {
		const { stdout } = await run(dialctl, ['-gatt', 'battery'], { windowsHide: true, timeout: 35000 })
		return { d100h: JSON.parse(stdout).percent }
	} catch (error) {
		const offline = /^(no paired Ulanzi Dial found in Windows|read battery: no matching reply|connect to Ulanzi Dial: device with the given address was not found|discover GATT service 1971: could not retrieve device services, operation failed with code 1)$/m
		if (error.killed || offline.test(error.stderr ?? '')) return { d100h: null }
		throw error
	}
}

async function readGamepads() {
	const { stdout } = await run('powershell.exe', ['-NoProfile', '-NonInteractive', '-File', gamepads], {
		windowsHide: true, timeout: 12000,
	})
	return JSON.parse(stdout)
}

async function readSteam() {
	const source = "'" + steamSource.replaceAll("'", "''") + "'"
	const command = "$ErrorActionPreference = 'Stop'; Add-Type -Path " + source +
		'; ConvertTo-Json -Compress -InputObject ([SteamBattery]::Read())'
	const encoded = Buffer.from(command, 'utf16le').toString('base64')
	const { stdout } = await run('powershell.exe', ['-NoProfile', '-NonInteractive', '-EncodedCommand', encoded], {
		windowsHide: true, timeout: 12000,
	})
	return { steam: JSON.parse(stdout) }
}

async function readLevels() {
	const levels = Object.fromEntries(Object.keys(tiles).map(id => [id, null]))
	const failures = []
	await Promise.all([readDial, readGamepads, readSteam].map(async read => {
		try {
			for (const [id, percent] of Object.entries(await read())) {
				if (!Object.hasOwn(levels, id) || (percent !== null && (!Number.isInteger(percent) || percent < 0 || percent > 100))) {
					throw new Error('Invalid battery reading: ' + id + '=' + percent)
				}
				levels[id] = percent
			}
		} catch (error) {
			failures.push(new Error(read.name + ': ' + error.message, { cause: error }))
		}
	}))
	await mkdir(dirname(output), { recursive: true })
	const pending = output + '.' + process.pid + '.tmp'
	await writeFile(pending, JSON.stringify(levels) + '\n')
	await rename(pending, output)
	return { levels, failures }
}

function tileImage(levels) {
	const known = Object.entries(tiles).filter(([id]) => levels[id] != null)
	const single = known.length === 1
	const threeRows = known.length > 4
	const cells = known.map(([id, label], index) => {
		const x = single || index === 4 ? 44 : 22 + (index % 2) * 44
		const y = single ? 25 : (known.length === 2 ? 32 : 12 + Math.floor(index / 2) * (threeRows ? 28 : 44))
		const labelSize = single ? 14 : (threeRows ? 9 : 11)
		const valueSize = single ? 34 : (threeRows ? 16 : 22)
		const valueY = y + (single ? 35 : (threeRows ? 15 : 23))
		return '<text x="' + x + '" y="' + y + '" fill="#8e8e93" font-size="' + labelSize + '">' + label + '</text>' +
			'<text x="' + x + '" y="' + valueY + '" fill="white" font-size="' + valueSize + '">' +
			levels[id] + '</text>'
	})
	const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="88" height="88">' +
		'<g font-family="JetBrains Mono, monospace" text-anchor="middle">' + cells.join('') + '</g></svg>'
	return 'data:image/svg+xml;base64,' + Buffer.from(svg).toString('base64')
}

async function publish(levels) {
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
		const response = await fetch(api + '/api/custom-variable/' + variable + '/value', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(tileImage(levels)),
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
	await publish(levels)
} catch (error) {
	failures.push(error)
}
if (failures.length) throw new AggregateError(failures, 'Battery refresh failed')
