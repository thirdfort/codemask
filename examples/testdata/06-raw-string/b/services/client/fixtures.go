package client

// subjectFixture is a YAML subject used by the import tests.
const subjectFixture = `
subject:
  kind: individual
  name: Jane Okafor
  email: jane.okafor@example.com
  address:
    line1: 14 Elm Row
    town: Edinburgh
  documents:
    - type: passport
      number: "533401827"
`

// importBatchSize is how many subjects one import call takes.
const importBatchSize = 100
