-- Bridge tables for multi-document support on property records, assessments, and intel
-- Allows permits to have multiple scans, assessments to have notices + appeals, etc.

-- Bridge: permits, inspections, certificates → multiple docs
CREATE TABLE property_records_media (
	record_id BIGINT NOT NULL REFERENCES property_records(id) ON DELETE CASCADE,
	media_id UUID NOT NULL REFERENCES media(id) ON DELETE CASCADE,
	position INTEGER,
	label TEXT,  -- 'application', 'approval', 'inspection_photo'
	PRIMARY KEY (record_id, media_id)
);

-- Bridge: assessments → notices, appeals, tax bills
CREATE TABLE property_assessments_media (
	assessment_id BIGINT NOT NULL REFERENCES property_assessments(id) ON DELETE CASCADE,
	media_id UUID NOT NULL REFERENCES media(id) ON DELETE CASCADE,
	position INTEGER,
	label TEXT,  -- 'notice', 'appeal', 'tax_bill'
	PRIMARY KEY (assessment_id, media_id)
);

-- Bridge: intel → screenshots, photos, court docs
CREATE TABLE property_intel_media (
	intel_id BIGINT NOT NULL REFERENCES property_intel(id) ON DELETE CASCADE,
	media_id UUID NOT NULL REFERENCES media(id) ON DELETE CASCADE,
	position INTEGER,
	label TEXT,  -- 'screenshot', 'photo_evidence', 'court_filing'
	PRIMARY KEY (intel_id, media_id)
);

CREATE INDEX idx_records_media_record ON property_records_media(record_id);
CREATE INDEX idx_assessments_media_assessment ON property_assessments_media(assessment_id);
CREATE INDEX idx_intel_media_intel ON property_intel_media(intel_id);
