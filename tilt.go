package transparencyprocessor

type tiltSpec struct {
	DataDisclosed []struct {
		Category string `json:"category"`
		Purposes []struct {
			Purpose string `json:"purpose"`
		} `json:"purposes"`
		LegalBases []struct {
			Reference string `json:"reference"`
		} `json:"legalBases"`
		LegitimateInterests []struct {
			Exists bool `json:"exists"`
		} `json:"legitimateInterests"`
		Storage []struct {
			Temporal []struct {
				TTL string `json:"ttl"`
			} `json:"temporal"`
		} `json:"storage"`
	} `json:"dataDisclosed"`
	AutomatedDecisionMaking struct {
		InUse         bool   `json:"inUse"`
		LogicInvolved string `json:"logicInvolved"`
	} `json:"automatedDecisionMaking"`
}
