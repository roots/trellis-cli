package db_opener

type DBCredentials struct {
	SSHUser    string `json:"web_user"`
	SSHHost    string `json:"ansible_host"`
	SSHPort    int    `json:"ansible_port"`
	DBUser     string `json:"db_user"`
	DBPassword string `json:"db_password"`
	DBHost     string `json:"db_host"`
	DBName     string `json:"db_name"`
	WPEnv      string `json:"wp_env"`
}

