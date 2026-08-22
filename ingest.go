package main

// postFacts will send extracted facts to Feather's ingest endpoint (POST cfg.IngestURL with a
// JSON array of Fact). Not implemented yet - that endpoint doesn't exist on the Feather side
// either. See Projects/Feather/Proactive/README.md in the Obsidian vault.
func postFacts(ingestURL string, facts []Fact) error {
	panic("postFacts: not implemented yet - see this file's comment")
}
