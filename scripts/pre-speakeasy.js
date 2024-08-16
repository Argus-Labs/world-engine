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

const privateKeyParam = {
  name: '_privateKey',
  in: 'query',
  schema: {
    type: 'string'
  },
}

const namespaceParam = {
  name: '_namespace',
  in: 'query',
  schema: {
    type: 'string'
  },
}

// sdk global params
data['x-speakeasy-globals'] = {
  parameters: [
    {
      ...privateKeyParam,
      'x-speakeasy-globals-hidden': true
    },
    {
      ...namespaceParam,
      'x-speakeasy-globals-hidden': true
    }
  ]
}

// routes that require signing
data.paths['/tx/game/{txName}'].post.parameters.push(privateKeyParam)
data.paths['/tx/game/{txName}'].post.parameters.push(namespaceParam)

// create-persona has no parameters so we need to set the initial empty array
data.paths['/tx/persona/create-persona'].post.parameters = []
data.paths['/tx/persona/create-persona'].post.parameters.push(privateKeyParam)
data.paths['/tx/persona/create-persona'].post.parameters.push(namespaceParam)

// use `additionalProperties` instead of `properties` for open maps in transaction request body
delete data.components.schemas['cardinal_server_handler.Transaction'].properties.body.properties
data.components.schemas['cardinal_server_handler.Transaction'].properties.body.additionalProperties = {}

try {
  fs.writeFileSync(openapiPath, JSON.stringify(data, null, 2))
  console.log('Updated openapi.json with speakeasy attributes')
} catch (error) {
  console.log('Error updating openapi.json:', error)
}
