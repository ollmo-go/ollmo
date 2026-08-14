package doc

import "time"

// MarkParsing / MarkParsed / MarkFailed / MarkReady are status helpers used
// by the worker. They are exposed on the service so the worker (which lives
// in the same binary) can update docs without reaching into the repo.
func (s *Service) MarkParsing(tenantID, id string) error {
	if err := s.repo.UpdateDocStatus(tenantID, id, StatusParsing, ""); err != nil {
		return err
	}
	if s.events != nil {
		s.events.Publish(DocEvent{TenantID: tenantID, DocID: id, Status: StatusParsing, At: time.Now()})
	}
	return nil
}

func (s *Service) MarkParsed(tenantID, id, parsedObjectKey string, chunkCount int) error {
	if err := s.repo.UpdateDocParsed(tenantID, id, parsedObjectKey, chunkCount); err != nil {
		return err
	}
	if err := s.repo.UpdateDocStatus(tenantID, id, StatusParsed, ""); err != nil {
		return err
	}
	if s.events != nil {
		s.events.Publish(DocEvent{TenantID: tenantID, DocID: id, Status: StatusParsed, At: time.Now()})
	}
	return nil
}

func (s *Service) MarkFailed(tenantID, id, reason string) error {
	if err := s.repo.UpdateDocStatus(tenantID, id, StatusFailed, reason); err != nil {
		return err
	}
	if s.events != nil {
		s.events.Publish(DocEvent{TenantID: tenantID, DocID: id, Status: StatusFailed, Error: reason, At: time.Now()})
	}
	return nil
}

func (s *Service) MarkReady(tenantID, id string) error {
	return s.repo.UpdateDocStatus(tenantID, id, StatusReady, "")
}

// MarkEmbedding / MarkEmbedded transition a doc into and out of the embedding
// phase. Used by the doc:embed worker.
func (s *Service) MarkEmbedding(tenantID, id string) error {
	if err := s.repo.UpdateDocStatus(tenantID, id, StatusEmbedding, ""); err != nil {
		return err
	}
	if s.events != nil {
		s.events.Publish(DocEvent{TenantID: tenantID, DocID: id, Status: StatusEmbedding, At: time.Now()})
	}
	return nil
}

func (s *Service) MarkEmbedded(tenantID, id string) error {
	if err := s.repo.UpdateDocStatus(tenantID, id, StatusReady, ""); err != nil {
		return err
	}
	if s.events != nil {
		s.events.Publish(DocEvent{TenantID: tenantID, DocID: id, Status: StatusReady, At: time.Now()})
	}
	return nil
}
