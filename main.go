// Package main is the entry point for the 3x-ui web panel application.
// It initializes the database, web server, and handles command-line operations for managing the panel.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	_ "unsafe"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/logger"
    "github.com/mhsanaei/3x-ui/v2/slave"
	"github.com/mhsanaei/3x-ui/v2/sub"
	"github.com/mhsanaei/3x-ui/v2/util/crypto"
	"github.com/mhsanaei/3x-ui/v2/web"
	"github.com/mhsanaei/3x-ui/v2/web/global"
	"github.com/mhsanaei/3x-ui/v2/web/service"

	"github.com/joho/godotenv"
	"github.com/op/go-logging"
)

// runWebServer initializes and starts the web server for the 3x-ui panel.
func runWebServer() {
	log.Printf("Starting %v %v", config.GetName(), config.GetVersion())

	switch config.GetLogLevel() {
	case config.Debug:
		logger.InitLogger(logging.DEBUG)
	case config.Info:
		logger.InitLogger(logging.INFO)
	case config.Notice:
		logger.InitLogger(logging.NOTICE)
	case config.Warning:
		logger.InitLogger(logging.WARNING)
	case config.Error:
		logger.InitLogger(logging.ERROR)
	default:
		logger.Fatalf("Unknown log level: %v", config.GetLogLevel())
	}

	logger.Infof("Starting %v %v", config.GetName(), config.GetVersion())
	logger.Info("Loading environment variables...")
	godotenv.Load()

	logger.Info("Initializing database...")
	err := database.InitDB(config.GetDBPath())
	if err != nil {
		logger.Fatalf("Error initializing database: %v", err)
	}

	logger.Info("Initializing web server...")
	var server *web.Server
	server = web.NewServer()
	global.SetWebServer(server)
	err = server.Start()
	if err != nil {
		logger.Fatalf("Error starting web server: %v", err)
	}
	logger.Info("Web server started successfully")

	logger.Info("Initializing subscription server...")
	var subServer *sub.Server
	subServer = sub.NewServer()
	global.SetSubServer(subServer)
	err = subServer.Start()
	if err != nil {
		logger.Fatalf("Error starting sub server: %v", err)
	}
	logger.Info("Subscription server started successfully")

	sigCh := make(chan os.Signal, 1)
	// Trap shutdown signals
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGTERM)
	for {
		sig := <-sigCh

		switch sig {
		case syscall.SIGHUP:
			logger.Info("Received SIGHUP signal. Restarting servers...")

			// --- FIX FOR TELEGRAM BOT CONFLICT (409): Stop bot before restart ---
			service.StopBot()
			// --

			err := server.Stop()
			if err != nil {
				logger.Debug("Error stopping web server:", err)
			}
			err = subServer.Stop()
			if err != nil {
				logger.Debug("Error stopping sub server:", err)
			}

			server = web.NewServer()
			global.SetWebServer(server)
			err = server.Start()
			if err != nil {
				logger.Fatalf("Error restarting web server: %v", err)
			}
			logger.Info("Web server restarted successfully")

			subServer = sub.NewServer()
			global.SetSubServer(subServer)
			err = subServer.Start()
			if err != nil {
				logger.Fatalf("Error restarting sub server: %v", err)
			}
			logger.Info("Subscription server restarted successfully")

		default:
			// --- FIX FOR TELEGRAM BOT CONFLICT (409) on full shutdown ---
			logger.Info("Shutting down servers...")
			service.StopBot()
			// ------------------------------------------------------------

			server.Stop()
			subServer.Stop()
			logger.Info("Servers shut down successfully")
			logger.CloseLogger()
			return
		}
	}
}

// resetSetting resets all panel settings to their default values.
func resetSetting() {
	logger.Info("Initializing database for settings reset...")
	err := database.InitDB(config.GetDBPath())
	if err != nil {
		logger.Errorf("Failed to initialize database: %v", err)
		fmt.Println("Failed to initialize database:", err)
		return
	}

	logger.Info("Resetting all panel settings...")
	settingService := service.SettingService{}
	err = settingService.ResetSettings()
	if err != nil {
		logger.Errorf("Failed to reset settings: %v", err)
		fmt.Println("Failed to reset settings:", err)
	} else {
		logger.Info("Settings successfully reset")
		fmt.Println("Settings successfully reset.")
	}
}

// showSetting displays the current panel settings if show is true.
func showSetting(show bool) {
	if show {
		settingService := service.SettingService{}
		port, err := settingService.GetPort()
		if err != nil {
			fmt.Println("get current port failed, error info:", err)
		}

		webBasePath, err := settingService.GetBasePath()
		if err != nil {
			fmt.Println("get webBasePath failed, error info:", err)
		}

		certFile, err := settingService.GetCertFile()
		if err != nil {
			fmt.Println("get cert file failed, error info:", err)
		}
		keyFile, err := settingService.GetKeyFile()
		if err != nil {
			fmt.Println("get key file failed, error info:", err)
		}

		userService := service.UserService{}
		userModel, err := userService.GetFirstUser()
		if err != nil {
			fmt.Println("get current user info failed, error info:", err)
		}

		if userModel.Username == "" || userModel.Password == "" {
			fmt.Println("current username or password is empty")
		}

		fmt.Println("current panel settings as follows:")
		if certFile == "" || keyFile == "" {
			fmt.Println("Warning: Panel is not secure with SSL")
		} else {
			fmt.Println("Panel is secure with SSL")
		}

		hasDefaultCredential := func() bool {
			return userModel.Username == "admin" && crypto.CheckPasswordHash(userModel.Password, "admin")
		}()

		fmt.Println("hasDefaultCredential:", hasDefaultCredential)
		fmt.Println("port:", port)
		fmt.Println("webBasePath:", webBasePath)
	}
}

// updateTgbotEnableSts enables or disables the Telegram bot notifications based on the status parameter.
func updateTgbotEnableSts(status bool) {
	settingService := service.SettingService{}
	currentTgSts, err := settingService.GetTgbotEnabled()
	if err != nil {
		fmt.Println(err)
		return
	}
	logger.Infof("current enabletgbot status[%v],need update to status[%v]", currentTgSts, status)
	if currentTgSts != status {
		err := settingService.SetTgbotEnabled(status)
		if err != nil {
			fmt.Println(err)
			return
		} else {
			logger.Infof("SetTgbotEnabled[%v] success", status)
		}
	}
}

// updateTgbotSetting updates Telegram bot settings including token, chat ID, and runtime schedule.
func updateTgbotSetting(tgBotToken string, tgBotChatid string, tgBotRuntime string) {
	logger.Info("Initializing database for Telegram bot settings update...")
	err := database.InitDB(config.GetDBPath())
	if err != nil {
		logger.Errorf("Error initializing database: %v", err)
		fmt.Println("Error initializing database:", err)
		return
	}

	settingService := service.SettingService{}

	if tgBotToken != "" {
		err := settingService.SetTgBotToken(tgBotToken)
		if err != nil {
			fmt.Printf("Error setting Telegram bot token: %v\n", err)
			return
		}
		logger.Info("Successfully updated Telegram bot token.")
	}

	if tgBotRuntime != "" {
		err := settingService.SetTgbotRuntime(tgBotRuntime)
		if err != nil {
			fmt.Printf("Error setting Telegram bot runtime: %v\n", err)
			return
		}
		logger.Infof("Successfully updated Telegram bot runtime to [%s].", tgBotRuntime)
	}

	if tgBotChatid != "" {
		err := settingService.SetTgBotChatId(tgBotChatid)
		if err != nil {
			fmt.Printf("Error setting Telegram bot chat ID: %v\n", err)
			return
		}
		logger.Info("Successfully updated Telegram bot chat ID.")
	}
}

// updateSetting updates various panel settings including port, credentials, base path, listen IP, and two-factor authentication.
func updateSetting(port int, username string, password string, webBasePath string, listenIP string, resetTwoFactor bool) {
	logger.Info("Initializing database for settings update...")
	err := database.InitDB(config.GetDBPath())
	if err != nil {
		logger.Errorf("Database initialization failed: %v", err)
		fmt.Println("Database initialization failed:", err)
		return
	}

	settingService := service.SettingService{}
	userService := service.UserService{}

	if port > 0 {
		logger.Infof("Setting panel port to %d...", port)
		err := settingService.SetPort(port)
		if err != nil {
			logger.Errorf("Failed to set port: %v", err)
			fmt.Println("Failed to set port:", err)
		} else {
			logger.Infof("Port set successfully: %d", port)
			fmt.Printf("Port set successfully: %v\n", port)
		}
	}

	if username != "" || password != "" {
		logger.Info("Updating panel credentials...")
		err := userService.UpdateFirstUser(username, password)
		if err != nil {
			logger.Errorf("Failed to update username and password: %v", err)
			fmt.Println("Failed to update username and password:", err)
		} else {
			logger.Info("Username and password updated successfully")
			fmt.Println("Username and password updated successfully")
		}
	}

	if webBasePath != "" {
		logger.Infof("Setting base URI path to %s...", webBasePath)
		err := settingService.SetBasePath(webBasePath)
		if err != nil {
			logger.Errorf("Failed to set base URI path: %v", err)
			fmt.Println("Failed to set base URI path:", err)
		} else {
			logger.Infof("Base URI path set successfully: %s", webBasePath)
			fmt.Println("Base URI path set successfully")
		}
	}

	if resetTwoFactor {
		logger.Info("Resetting two-factor authentication...")
		err := settingService.SetTwoFactorEnable(false)

		if err != nil {
			logger.Errorf("Failed to reset two-factor authentication: %v", err)
			fmt.Println("Failed to reset two-factor authentication:", err)
		} else {
			settingService.SetTwoFactorToken("")
			logger.Info("Two-factor authentication reset successfully")
			fmt.Println("Two-factor authentication reset successfully")
		}
	}

	if listenIP != "" {
		logger.Infof("Setting listen IP to %s...", listenIP)
		err := settingService.SetListen(listenIP)
		if err != nil {
			logger.Errorf("Failed to set listen IP: %v", err)
			fmt.Println("Failed to set listen IP:", err)
		} else {
			logger.Infof("Listen IP set successfully: %s", listenIP)
			fmt.Printf("listen %v set successfully", listenIP)
		}
	}
}

// updateCert updates the SSL certificate files for the panel.
func updateCert(publicKey string, privateKey string) {
	logger.Info("Initializing database for certificate update...")
	err := database.InitDB(config.GetDBPath())
	if err != nil {
		logger.Errorf("Database initialization failed: %v", err)
		fmt.Println(err)
		return
	}

	if (privateKey != "" && publicKey != "") || (privateKey == "" && publicKey == "") {
		logger.Info("Updating SSL certificates...")
		settingService := service.SettingService{}
		err = settingService.SetCertFile(publicKey)
		if err != nil {
			logger.Errorf("Set certificate public key failed: %v", err)
			fmt.Println("set certificate public key failed:", err)
		} else {
			logger.Info("Certificate public key set successfully")
			fmt.Println("set certificate public key success")
		}

		err = settingService.SetKeyFile(privateKey)
		if err != nil {
			logger.Errorf("Set certificate private key failed: %v", err)
			fmt.Println("set certificate private key failed:", err)
		} else {
			logger.Info("Certificate private key set successfully")
			fmt.Println("set certificate private key success")
		}

		err = settingService.SetSubCertFile(publicKey)
		if err != nil {
			logger.Errorf("Set certificate for subscription public key failed: %v", err)
			fmt.Println("set certificate for subscription public key failed:", err)
		} else {
			logger.Info("Subscription certificate public key set successfully")
			fmt.Println("set certificate for subscription public key success")
		}

		err = settingService.SetSubKeyFile(privateKey)
		if err != nil {
			logger.Errorf("Set certificate for subscription private key failed: %v", err)
			fmt.Println("set certificate for subscription private key failed:", err)
		} else {
			logger.Info("Subscription certificate private key set successfully")
			fmt.Println("set certificate for subscription private key success")
		}
	} else {
		logger.Warning("Both public and private key should be entered")
		fmt.Println("both public and private key should be entered.")
	}
}

// GetCertificate displays the current SSL certificate settings if getCert is true.
func GetCertificate(getCert bool) {
	if getCert {
		settingService := service.SettingService{}
		certFile, err := settingService.GetCertFile()
		if err != nil {
			fmt.Println("get cert file failed, error info:", err)
		}
		keyFile, err := settingService.GetKeyFile()
		if err != nil {
			fmt.Println("get key file failed, error info:", err)
		}

		fmt.Println("cert:", certFile)
		fmt.Println("key:", keyFile)
	}
}

// GetListenIP displays the current panel listen IP address if getListen is true.
func GetListenIP(getListen bool) {
	if getListen {

		settingService := service.SettingService{}
		ListenIP, err := settingService.GetListen()
		if err != nil {
			logger.Errorf("Failed to retrieve listen IP: %v", err)
			fmt.Printf("Failed to retrieve listen IP: %v", err)
			return
		}

		fmt.Println("listenIP:", ListenIP)
	}
}

// migrateDb performs database migration operations for the 3x-ui panel.
func migrateDb() {
	logger.InitLogger(logging.INFO)
	inboundService := service.InboundService{}

	logger.Info("Initializing database for migration...")
	err := database.InitDB(config.GetDBPath())
	if err != nil {
		logger.Fatalf("Database initialization failed: %v", err)
	}
	fmt.Println("Start migrating database...")
	logger.Info("Starting database migration...")
	inboundService.MigrateDB()
	fmt.Println("Migration done!")
	logger.Info("Database migration completed successfully")
}

// main is the entry point of the 3x-ui application.
// It parses command-line arguments to run the web server, migrate database, or update settings.
func main() {
	if len(os.Args) < 2 {
		runWebServer()
		return
	}

	var showVersion bool
	flag.BoolVar(&showVersion, "v", false, "show version")

	runCmd := flag.NewFlagSet("run", flag.ExitOnError)

	settingCmd := flag.NewFlagSet("setting", flag.ExitOnError)

    slaveCmd := flag.NewFlagSet("slave", flag.ExitOnError)
    masterUrl := slaveCmd.String("master", "", "Master Server URL")
    slaveSecret := slaveCmd.String("secret", "", "Slave Secret")

	var port int
	var username string
	var password string
	var webBasePath string
	var listenIP string
	var getListen bool
	var webCertFile string
	var webKeyFile string
	var tgbottoken string
	var tgbotchatid string
	var enabletgbot bool
	var tgbotRuntime string
	var reset bool
	var show bool
	var getCert bool
	var resetTwoFactor bool
	settingCmd.BoolVar(&reset, "reset", false, "Reset all settings")
	settingCmd.BoolVar(&show, "show", false, "Display current settings")
	settingCmd.IntVar(&port, "port", 0, "Set panel port number")
	settingCmd.StringVar(&username, "username", "", "Set login username")
	settingCmd.StringVar(&password, "password", "", "Set login password")
	settingCmd.StringVar(&webBasePath, "webBasePath", "", "Set base path for Panel")
	settingCmd.StringVar(&listenIP, "listenIP", "", "set panel listenIP IP")
	settingCmd.BoolVar(&resetTwoFactor, "resetTwoFactor", false, "Reset two-factor authentication settings")
	settingCmd.BoolVar(&getListen, "getListen", false, "Display current panel listenIP IP")
	settingCmd.BoolVar(&getCert, "getCert", false, "Display current certificate settings")
	settingCmd.StringVar(&webCertFile, "webCert", "", "Set path to public key file for panel")
	settingCmd.StringVar(&webKeyFile, "webCertKey", "", "Set path to private key file for panel")
	settingCmd.StringVar(&tgbottoken, "tgbottoken", "", "Set token for Telegram bot")
	settingCmd.StringVar(&tgbotRuntime, "tgbotRuntime", "", "Set cron time for Telegram bot notifications")
	settingCmd.StringVar(&tgbotchatid, "tgbotchatid", "", "Set chat ID for Telegram bot notifications")
	settingCmd.BoolVar(&enabletgbot, "enabletgbot", false, "Enable notifications via Telegram bot")

	oldUsage := flag.Usage
	flag.Usage = func() {
		oldUsage()
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("    run            run web panel")
		fmt.Println("    migrate        migrate form other/old x-ui")
		fmt.Println("    setting        set settings")
	}

	flag.Parse()
	if showVersion {
		fmt.Println(config.GetVersion())
		return
	}

	switch os.Args[1] {
	case "run":
		err := runCmd.Parse(os.Args[2:])
		if err != nil {
			fmt.Println(err)
			return
		}
		runWebServer()
    case "slave":
        // Initialize logger for slave mode
        logger.InitLogger(logging.INFO)
        
        // Support both positional arguments and flags
        // Usage: 3x-ui slave <master_url> <secret>
        // Or: 3x-ui slave --master <url> --secret <key>
        var masterUrlVal, secretVal string
        
        if len(os.Args) >= 4 && !strings.HasPrefix(os.Args[2], "-") {
            // Positional arguments
            masterUrlVal = os.Args[2]
            secretVal = os.Args[3]
        } else {
            // Flag arguments
            err := slaveCmd.Parse(os.Args[2:])
            if err != nil {
                fmt.Println(err)
                return
            }
            masterUrlVal = *masterUrl
            secretVal = *slaveSecret
        }
        
        if masterUrlVal == "" || secretVal == "" {
            fmt.Println("Error: master URL and secret are required for slave mode")
            fmt.Println("Usage: 3x-ui slave <master_url> <secret>")
            fmt.Println("   Or: 3x-ui slave --master <url> --secret <key>")
            return
        }
        slave.Run(masterUrlVal, secretVal)
	case "migrate":
		migrateDb()
	case "setting":
		// Initialize logger for setting commands
		logger.InitLogger(logging.INFO)
		
		err := settingCmd.Parse(os.Args[2:])
		if err != nil {
			fmt.Println(err)
			return
		}
		if reset {
			resetSetting()
		} else {
			updateSetting(port, username, password, webBasePath, listenIP, resetTwoFactor)
		}
		if show {
			showSetting(show)
		}
		if getListen {
			GetListenIP(getListen)
		}
		if getCert {
			GetCertificate(getCert)
		}
		if (tgbottoken != "") || (tgbotchatid != "") || (tgbotRuntime != "") {
			updateTgbotSetting(tgbottoken, tgbotchatid, tgbotRuntime)
		}
		if enabletgbot {
			updateTgbotEnableSts(enabletgbot)
		}
	case "cert":
		// Initialize logger for cert commands
		logger.InitLogger(logging.INFO)
		
		err := settingCmd.Parse(os.Args[2:])
		if err != nil {
			fmt.Println(err)
			return
		}
		if reset {
			updateCert("", "")
		} else {
			updateCert(webCertFile, webKeyFile)
		}
	default:
		fmt.Println("Invalid subcommands")
		fmt.Println()
		runCmd.Usage()
		fmt.Println()
		settingCmd.Usage()
	}
}
