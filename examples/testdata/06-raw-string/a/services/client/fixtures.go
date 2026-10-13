package client

// subjectFixture is a YAML subject used by the import tests.
const subjectFixture = `
subject:
  kind: individual
  name: Test Subject
  email: subject@example.com
  address:
    line1: 1 Test Street
    town: Testville
  documents:
    - type: passport
      number: "000000000"
`

// importBatchSize is how many subjects one import call takes.
const importBatchSize = 50
