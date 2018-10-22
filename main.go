package main

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"path/filepath"
	"os"
	"flag"
	"strings"

	//v1 "k8s.io/api/core/v1"
	//appsv1 "k8s.io/api/apps/v1"
	//apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	//"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

type Images struct {
	Name string `json:"name"`
	Image string `json:"image"`
}

type Apps struct {
	Name string `json:"name"`
	Namespace string `json:"namespace"`
	Replica int32 `json:"replica"`
	Chart string `json:"chart,omitempty"`
	Images []Images `json:"images,omitempty"`
}

type Nodes struct {
	Total int `json:"total"`
	Master int `json:"master"`
	Worker int `json:"worker"`
}

type Infra struct {
	Version string `json:"version"`
	Plateform string `json:"plateform"`
	Region string `json:"region,omitempty"`
	Nodes Nodes `json:"nodes,omitempty"`
}

type Audit struct {
	Apps []Apps `json:"apps,omitempty"`
	Infra Infra `json:"infra,omitempty"`
}
var NAMESPACES = "kube-system,monitoring,logging,backup,claranet,nginx-ingress"

func main() {
	clientset := getcnx()
	apps := getapps(clientset)
	infra := getinfra(clientset)
	audit := Audit{apps,infra}
	res, _ := json.Marshal(audit)
	fmt.Println(string(res))
}

func getapps(clientset *kubernetes.Clientset) []Apps {
	var apps []Apps
	var images []Images
	for _, ns := range strings.Split(NAMESPACES, ",") {
		//Get Deployment
		list, _ := clientset.AppsV1().Deployments(ns).List(metav1.ListOptions{})
		for _, d := range list.Items {
			chart := d.ObjectMeta.Labels["chart"]
			containers := d.Spec.Template.Spec.Containers
			for _, c := range containers {
				images = append(images, Images{Name: c.Name, Image: c.Image})
			}
			apps = append(apps, Apps{Name: d.Name, Namespace: ns, Replica: *d.Spec.Replicas, Images: images, Chart: chart})
			images = nil
		}
		//Get StatefulSet
		lists, _ := clientset.AppsV1().StatefulSets(ns).List(metav1.ListOptions{})
		for _, s := range lists.Items {
			chart := s.ObjectMeta.Labels["chart"]
			containers := s.Spec.Template.Spec.Containers
			for _, c := range containers {
				images = append(images, Images{Name: c.Name, Image: c.Image})
			}
			apps = append(apps, Apps{Name: s.Name, Namespace: ns, Replica: *s.Spec.Replicas, Images: images, Chart: chart})
			images = nil
		}

	}
	return apps
}

func getinfra(clientset *kubernetes.Clientset) Infra {
	infra := make(map[string]string)
	nodes := make(map[string]int)
	// Get Master Kubernetes version
	sVer, err := clientset.ServerVersion()
	if err != nil {
			fmt.Printf("failed to get Server version %v", err)
	}
	version := sVer.GitVersion
	infra["Version"] = version
	// Get all nodes
	nodeslist, err := clientset.CoreV1().Nodes().List(metav1.ListOptions{})
	nodes_total := len(nodeslist.Items)
	nodes["Total"] = nodes_total
	for _, hostname := range nodeslist.Items {
		hostname := hostname.Name
		matchaws, _ := regexp.MatchString(".+\\.(.+)\\.compute\\.internal", hostname)
		matchgke, _ := regexp.MatchString(".+-gke\\..+", version)
		node, err := clientset.CoreV1().Nodes().Get(hostname, metav1.GetOptions{})
		if err != nil {
			fmt.Printf("failed to get node %v", err)
		}
		region := node.ObjectMeta.Labels["failure-domain.beta.kubernetes.io/region"]
		infra["Region"] = region
		if matchaws {
			masterlist, _ := clientset.CoreV1().Nodes().List(metav1.ListOptions{LabelSelector:"kubernetes.io/role=master"})
			nodelist, _ := clientset.CoreV1().Nodes().List(metav1.ListOptions{LabelSelector:"kubernetes.io/role=node"})
			infra["Plateform"] = "aws"
			nodes["Master"] = len(masterlist.Items)
			nodes["Worker"] = len(nodelist.Items)
		} else if matchgke {
			infra["Plateform"] = "gke"
			nodes["Master"] = 0
			nodes["Worker"] = nodes_total
		} else {
			infra["Plateform"] = "unknown"
			masterlist, _ := clientset.CoreV1().Nodes().List(metav1.ListOptions{LabelSelector:"kubernetes.io/role=master"})
			nodelist, _ := clientset.CoreV1().Nodes().List(metav1.ListOptions{LabelSelector:"kubernetes.io/role=node"})
			nodes["Master"] = len(masterlist.Items)
			nodes["Worker"] = len(nodelist.Items)
		}
		break
	}
	return Infra{infra["Version"], infra["Plateform"], infra["Region"], Nodes{nodes["Total"], nodes["Master"], nodes["Worker"]}}
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func getcnx() *kubernetes.Clientset {
	var kubeconfig *string
	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}
	//config, err := rest.InClusterConfig()
	//if err != nil {
	//	panic(err.Error())
	//}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	if err != nil {
		log.Fatal(err)
	}
	return clientset
}

