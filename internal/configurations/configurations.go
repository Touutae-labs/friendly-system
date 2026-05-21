package configurations

type ServerConfig struct {
	Port             string `koanf:"port"`
	Title            string `koanf:"title"`
	Version          string `koanf:"version"`
	MaxPayloadSizeKB int    `koanf:"max_payload_size_kb"`
	TimeoutSeconds   int    `koanf:"timeout_seconds"`
	BaseURL          string `koanf:"base_url"`
}

type DatabaseConfig struct {
	Path string `koanf:"path"`
}

type LimitConfig struct {
	DailyLimit string `koanf:"daily_limit"`
}

type QuoteConfig struct {
	PriceTolerance string `koanf:"price_tolerance"`
	SpreadMargin   string `koanf:"spread_margin"`
}

type OrderValConfig struct {
	QuantityIncrement   string `koanf:"quantity_increment"`
	MaxQuantity         string `koanf:"max_quantity"`
	MaxPrice            string `koanf:"max_price"`
	MaxCustomerIDLength int    `koanf:"max_customer_id_length"`
}

type Config struct {
	Server   ServerConfig   `koanf:"server"`
	Database DatabaseConfig `koanf:"database"`
	Limit    LimitConfig    `koanf:"limit"`
	Quote    QuoteConfig    `koanf:"quote"`
	OrderVal OrderValConfig `koanf:"orderval"`
}
