<!-- Source: https://www.legendkeeper.com/developers/quickstart/ — converted from HTML; see FETCHED.md. Do not hand-edit. -->

# @legendkeeper/sdk

A TypeScript client for the LegendKeeper API. It is generated from the v3 OpenAPI spec and has no runtime dependencies. It runs on Node 20 and later and in edge runtimes. Use it from server-side code: an API key is a secret, and the API does not accept requests from browser pages on other sites.

This page shows how to install the client, make a first call, and write a first page. For every other topic, see the [SDK guide](https://www.legendkeeper.com/developers/guide/).

## Install

```sh
npm install @legendkeeper/sdk
```

## Get an API key

In LegendKeeper, open the project and go to Project Settings → Developers → API Keys. Name the key, choose its role, and create it. The secret is shown only once, so copy it then. API keys require the project owner to be on a paid plan. A trial does not count.

A key belongs to one project and acts as the user who created it. Its role is Member or Admin. The key acts with the lower of that role and its creator's current role in the project.

For every rule, see [Get an API key](https://www.legendkeeper.com/developers/guide/#get-an-api-key) in the guide.

Set the key in the environment:

```sh
export LEGENDKEEPER_API_KEY=lk_...
```

## Make a first call

Save this as `first-call.ts` and run it with `npx tsx first-call.ts`. The examples use top-level `await`, so they run as ES modules. Set `"type": "module"` in your `package.json`, or name the file `first-call.mts`.

```ts
import { LegendKeeper } from '@legendkeeper/sdk'

const lk = new LegendKeeper({ apiKey: process.env.LEGENDKEEPER_API_KEY! })

const project = await lk.project()
console.log(`Connected to ${project.name} (${project.id})`)

const resource = await lk.resources.create({ name: 'The Tree Fort' })
console.log(`Created ${resource.name} (${resource.id})`)

await lk.resources.delete({ resourceId: resource.id })
```

`lk.project()` returns the project the key belongs to. Every operation that takes a `projectId` fills it in from the key, so the example passes only `resourceId`.

## Write a page

```ts
import { LegendKeeper } from '@legendkeeper/sdk'
import { bold, callout, h, p, plainText, ul } from '@legendkeeper/sdk/blocks'

const lk = new LegendKeeper({ apiKey: process.env.LEGENDKEEPER_API_KEY! })

const kingdom = await lk.resources.create({ name: 'Candy Kingdom' })
try {
  const page = kingdom.documents[0]!
  const doc = { resourceId: kingdom.id, documentId: page.id }

  await lk.documents.replaceBlocks(doc, {
    blocks: [
      h(1, 'Candy Kingdom'),
      p('Ruled by ', bold('Princess Bubblegum'), '. Home of the Candy People.'),
      ul('Candy Castle', 'Candy Village'),
      callout('note', ['The Banana Guards guard the castle.']),
    ],
  })

  const read = await lk.documents.getBlocks(doc)
  console.log(read.blocks.map((block) => block.kind).join(', '))
  console.log(plainText(read))
} finally {
  await lk.resources.delete({ resourceId: kingdom.id })
}
```

A new resource has one blank document, `documents[0]`, and the first write gives it a type. The example writes four blocks to that document with the builders from `@legendkeeper/sdk/blocks`, which makes it a page. It then reads the page back, prints its block kinds and its text, and deletes the resource.

## Next

- [Client options](https://www.legendkeeper.com/developers/guide/#client-options): timeouts, retries, a custom `fetch` and `signal`.
- [Errors](https://www.legendkeeper.com/developers/guide/#errors): the error classes, the error codes and `requestId`.
- [Rate limits and retries](https://www.legendkeeper.com/developers/guide/#rate-limits-and-retries) and [idempotent writes](https://www.legendkeeper.com/developers/guide/#idempotent-writes).
- [Pagination](https://www.legendkeeper.com/developers/guide/#pagination) and [versioned writes](https://www.legendkeeper.com/developers/guide/#versioned-writes).
- [Writing content](https://www.legendkeeper.com/developers/guide/#writing-content): every block builder, block operations, key-value rows and infoboxes.
- [Namespaces and operations](https://www.legendkeeper.com/developers/guide/#namespaces-and-operations): resources, documents, properties, boards, timelines, calendars, maps, assets, search, tags, members and projects.
- [The CLI](https://www.legendkeeper.com/developers/guide/#the-cli), [the escape hatch](https://www.legendkeeper.com/developers/guide/#the-escape-hatch) and [TypeScript types](https://www.legendkeeper.com/developers/guide/#typescript-types).
- [Worked example](https://www.legendkeeper.com/developers/guide/#worked-example): a small Land of Ooo with pages, a map and a timeline.
- [API reference](https://www.legendkeeper.com/developers/api/), [SDK reference](https://www.legendkeeper.com/developers/sdk/) and the [OpenAPI spec](https://www.legendkeeper.com/api/v3/openapi.json).
