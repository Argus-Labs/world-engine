<!--
Guiding Principles:

Changelogs are for humans, not machines.
There should be an entry for every single version.
The same types of changes should be grouped.
Versions and sections should be linkable.
The latest version comes first.
The release date of each version is displayed.
Mention whether you follow Semantic Versioning.

Usage:

Change log entries are to be added to the Unreleased section under the
appropriate stanza (see below). Each entry is required to include a tag and
the Github issue reference in the following format:

* (<tag>) \#<issue-number> message

The tag should consist of where the change is being made ex. (cardinal), (evm)
The issue numbers will later be link-ified during the release process so you do
not have to worry about including a link manually, but you can if you wish.

Types of changes (Stanzas):

"Features" for new features.
"Improvements" for changes in existing functionality.
"Deprecated" for soon-to-be removed features.
"Bug Fixes" for any bug fixes.
"Client Breaking" for breaking API routes, gRPC routes, or Cardinal SDK code.
"API Breaking" for breaking exported APIs used by developers building on World Engine.
"State Machine Breaking" for any changes that result in a different game state given same genesis state and tx list.
"Runtime Breaking" for any changes that will cause the application to crash at runtime unless new changes are applied.
Ref: https://keepachangelog.com/en/1.0.0/
-->

# Changelog

## [Unreleased]

### Features

- (cardinal) #WORLD-671: Add support for exporting custom metrics

- (cardinal) #WORLD-642: Log level is settable via the CARDINAL_LOG_LEVEL environment variable.

- (cardinal) #WORLD-627: timestamps are now accessible via WorldContext.

- (nakama) #WORLD-651: Support saving game data to Nakama storage.

### Improvements

- (nakama) #WORLD-659: Bump nakama to version 1.30 (which includes go 1.21) and add unit tests based on mockery.

- (cardinal) #WORLD-668: Log tick timing information into a single log line.

- (cardinal) #WORLD-643: Instead of making sure nonce values are strictly increasing, use a Redis set to track every used nonce.

- (cardinal) #WORLD-702: Cardinal's transactions are now sent at the end of a tick, instead of one by one at the server handler.

- (nakama) GDEV-1024: Nakama will automatically attempt to reconnect a websocket when cardinal is restarted.

### Deprecated

### Bug Fixes

- (cardinal) #WORLD-653: Fix memory leak in ECB.

- (nakama) #WORLD-643: Reject a persona tag request if it seems like cardinal didn't actually get the corresponding transaction.

### Client Breaking

### API Breaking

### State Machine Breaking

### Runtime Breaking

- (cardinal) #WORLD-676: Adapter instantiation is now automatic in production mode, and requires setting two new env vars: `BASE_SHARD_SEQUENCER_ADDRESS` and `BASE_SHARD_QUERY_ADDRESS`.
