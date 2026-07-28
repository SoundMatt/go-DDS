// Command operator runs the go-DDS Kubernetes operator (ROADMAP.md,
// Milestone 15, "Kubernetes Operator"):
//
//   - Watches DDSParticipant and DDSDomain custom resources.
//   - Serves a mutating admission webhook that injects DDS_* environment
//     variables into pods annotated with dds.soundmatt.io/participant.
//   - Reconciles DDSDomain resources into NetworkPolicy objects providing
//     domain-per-namespace isolation.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/SoundMatt/go-DDS/k8s/operator/api/v1alpha1"
	"github.com/SoundMatt/go-DDS/k8s/operator/internal/cache"
	"github.com/SoundMatt/go-DDS/k8s/operator/internal/controller"
	"github.com/SoundMatt/go-DDS/k8s/operator/internal/webhook"
)

var namespaceGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}

func main() {
	var (
		kubeconfig  = flag.String("kubeconfig", os.Getenv("KUBECONFIG"), "path to a kubeconfig; empty uses in-cluster config")
		webhookAddr = flag.String("webhook-addr", ":9443", "address the mutating admission webhook listens on")
		healthAddr  = flag.String("health-addr", ":8081", "address serving /healthz and /readyz")
		certFile    = flag.String("tls-cert-file", "/etc/go-dds-operator/tls/tls.crt", "webhook server TLS certificate")
		keyFile     = flag.String("tls-key-file", "/etc/go-dds-operator/tls/tls.key", "webhook server TLS private key")
		workers     = flag.Int("workers", 2, "number of DDSDomain reconcile workers")
	)
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(*kubeconfig, *webhookAddr, *healthAddr, *certFile, *keyFile, *workers, log); err != nil {
		log.Error("go-dds-operator exited with error", "error", err)
		os.Exit(1)
	}
}

func run(kubeconfig, webhookAddr, healthAddr, certFile, keyFile string, workers int, log *slog.Logger) error {
	cfg, err := loadConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("loading kubeconfig: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("building dynamic client: %w", err)
	}
	typedClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("building typed client: %w", err)
	}

	factory := dynamicinformer.NewDynamicSharedInformerFactory(dynClient, 10*time.Minute)
	participantInformer := factory.ForResource(v1alpha1.ParticipantGVR).Informer()
	domainInformer := factory.ForResource(v1alpha1.DomainGVR).Informer()
	namespaceInformer := factory.ForResource(namespaceGVR).Informer()

	participants := cache.NewParticipants()
	domains := cache.NewDomains()
	namespaceDomains := cache.NewNamespaceDomains()

	if _, err := participantInformer.AddEventHandler(participants.EventHandler()); err != nil {
		return fmt.Errorf("registering DDSParticipant event handler: %w", err)
	}
	if _, err := domainInformer.AddEventHandler(domains.EventHandler()); err != nil {
		return fmt.Errorf("registering DDSDomain event handler: %w", err)
	}
	if _, err := namespaceInformer.AddEventHandler(namespaceDomains.EventHandler()); err != nil {
		return fmt.Errorf("registering Namespace event handler: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	factory.Start(ctx.Done())
	log.Info("waiting for informer caches to sync")
	for gvr, synced := range factory.WaitForCacheSync(ctx.Done()) {
		if !synced {
			return fmt.Errorf("cache for %v never synced", gvr)
		}
	}
	log.Info("informer caches synced",
		"participants", participants.Len(), "domains", domains.Len(), "namespaceBindings", namespaceDomains.Len())

	reconciler := &controller.DomainReconciler{Client: typedClient, Informer: domainInformer, Log: log}
	go func() {
		if err := reconciler.Run(ctx, workers); err != nil {
			log.Error("DDSDomain reconciler stopped", "error", err)
		}
	}()

	mutator := &webhook.Mutator{
		Participants: participants,
		Domains:      domains,
		NamespaceMap: namespaceDomains,
		Log:          log,
	}

	healthSrv := startHealthServer(healthAddr, log)
	webhookSrv, err := startWebhookServer(webhookAddr, certFile, keyFile, mutator, log)
	if err != nil {
		return fmt.Errorf("starting webhook server: %w", err)
	}

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = healthSrv.Shutdown(shutdownCtx)
	_ = webhookSrv.Shutdown(shutdownCtx)
	return nil
}

func loadConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg, nil
	}
	// Fall back to the default kubeconfig loading rules (e.g. local
	// development outside a cluster with no --kubeconfig/$KUBECONFIG set).
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{},
	).ClientConfig()
}

func startHealthServer(addr string, log *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("health server stopped", "error", err)
		}
	}()
	return srv
}

func startWebhookServer(addr, certFile, keyFile string, mutator *webhook.Mutator, log *slog.Logger) (*http.Server, error) {
	mux := http.NewServeMux()
	mux.Handle("/mutate-pods", mutator)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}
	go func() {
		log.Info("webhook server listening", "addr", addr)
		if err := srv.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
			log.Error("webhook server stopped", "error", err)
		}
	}()
	return srv, nil
}
