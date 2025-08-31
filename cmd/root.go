package cmd

import (
	"github.com/go-logr/zapr"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"janus-ingress-controller/internal/controller/config"
	"janus-ingress-controller/internal/manager"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
)

func GetRootCmd() *cobra.Command {
	root := newJANUSIngressController()
	return root
}

func newJANUSIngressController() *cobra.Command {
	cfg := config.ControllerConfig
	var configPath string
	cmd := &cobra.Command{
		Use:  "janus-ingress-controller [command]",
		Long: "",
		RunE: func(cmd *cobra.Command, args []string) error {
			if configPath != "" {
				c, err := config.GetConfigFromFile(configPath)
				if err != nil {
					return err
				}
				cfg = c
				config.SetControllerConfig(c)
			}

			logLevel, err := zapcore.ParseLevel(cfg.LogLevel)
			if err != nil {
				return err
			}

			// controllers log
			core := zapcore.NewCore(
				zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
				zapcore.AddSync(zapcore.Lock(os.Stderr)),
				logLevel,
			)
			logger := zapr.NewLogger(zap.New(core, zap.AddCaller()))

			logger.Info("controller initialized", "configuration", cfg)
			ctrl.SetLogger(logger.WithName("controller-runtime"))

			ctx := ctrl.LoggerInto(cmd.Context(), logger)
			return manager.Run(ctx, logger)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config-path", "c", "", "configuration file path for janus-ingress-controller")
	cmd.Flags().StringVar(&cfg.MetricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	cmd.Flags().StringVar(&cfg.ProbeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	cmd.Flags().StringVar(&cfg.LogLevel, "log-level", config.DefaultLogLevel, "The log level for janus-ingress-controller")
	cmd.Flags().StringVar(&cfg.ControllerName, "controller-name", config.DefaultControllerName, "The name of the controller")

	return cmd
}
