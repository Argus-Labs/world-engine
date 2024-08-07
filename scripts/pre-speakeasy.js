const fs = require('fs')

const openapiPath = process.argv[2]

const file = fs.readFileSync(openapiPath)
const data = JSON.parse(file)

// Rename methods
data.paths['/cql'].post['x-speakeasy-name-override'] = 'queryCql'
data.paths['/debug/state'].post['x-speakeasy-name-override'] = 'getDebugState'
data.paths['/health'].get['x-speakeasy-name-override'] = 'getHealth'
data.paths['/query/game/{queryName}'].post['x-speakeasy-name-override'] = 'query'
data.paths['/query/receipts/list'].post['x-speakeasy-name-override'] = 'getReceipts'
data.paths['/tx/game/{txName}'].post['x-speakeasy-name-override'] = 'transact'
data.paths['/tx/persona/create-persona'].post['x-speakeasy-name-override'] = 'createPersona'
data.paths['/world'].get['x-speakeasy-name-override'] = 'getWorld'

// hide methods
data.paths['/events'].get['x-speakeasy-ignore'] = true
data.paths['/query/{queryGroup}/{queryName}'].post['x-speakeasy-ignore'] = true
data.paths['/tx/{txGroup}/{txName}'].post['x-speakeasy-ignore'] = true

// sdk global params
data['x-speakeasy-globals'] = {
  parameters: [
    {
      name: 'privateKey',
      in: 'query',
      schema: {
        type: 'string'
      },
      'x-speakeasy-globals-hidden': true
    }
  ]
}

try {
  fs.writeFileSync(openapiPath, JSON.stringify(data, null, 2))
  console.log('Updated openapi.json with speakeasy attributes')
} catch (error) {
  console.log('Error updating openapi.json:', error)
}
