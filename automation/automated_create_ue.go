package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"path/filepath"

	"github.com/chromedp/chromedp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type WebUIAutomation struct {
	clientset   *kubernetes.Clientset
	minikubeIP  string
	webuiPort   int32
	webuiURL    string
	imsiCounter int // Counter to track IMSI changes
}

func main() {
	automation := &WebUIAutomation{}

	// Initialize Kubernetes client
	if err := automation.initKubernetesClient(); err != nil {
		log.Fatalf("Failed to initialize Kubernetes client: %v", err)
	}

	// Get minikube IP
	if err := automation.getMinikubeIP(); err != nil {
		log.Fatalf("Failed to get minikube IP: %v", err)
	}

	// Get webui service nodeport
	if err := automation.getWebuiServicePort(); err != nil {
		log.Fatalf("Failed to get webui service port: %v", err)
	}

	// Construct webui URL (base URL for login)
	automation.webuiURL = fmt.Sprintf("http://%s:%d", automation.minikubeIP, automation.webuiPort)
	log.Printf("WebUI URL: %s", automation.webuiURL)

	// Automate Chrome browser
	if err := automation.automateBrowser(); err != nil {
		log.Fatalf("Failed to automate browser: %v", err)
	}

	log.Println("Automation completed successfully!")
}

func (w *WebUIAutomation) initKubernetesClient() error {
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	// Use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to build config: %w", err)
	}

	// Create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	w.clientset = clientset
	log.Println("Successfully connected to Kubernetes cluster")
	return nil
}

func (w *WebUIAutomation) getMinikubeIP() error {
	// Try to get minikube IP using minikube command
	cmd := exec.Command("minikube", "ip")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get minikube IP: %w", err)
	}

	w.minikubeIP = strings.TrimSpace(string(output))
	log.Printf("Minikube IP: %s", w.minikubeIP)
	return nil
}

func (w *WebUIAutomation) getWebuiServicePort() error {
	// First, try to find the webui service across all namespaces
	namespaces := []string{"", "default", "free5gc", "open5gs", "kube-system"}

	for _, namespace := range namespaces {
		log.Printf("Searching for webui service in namespace: %s", namespace)

		service, err := w.clientset.CoreV1().Services(namespace).Get(context.TODO(), "webui", metav1.GetOptions{})
		if err != nil {
			log.Printf("Service 'webui' not found in namespace '%s': %v", namespace, err)
			continue
		}

		log.Printf("Found webui service in namespace: %s", namespace)

		// Find port 5000 and get its NodePort
		for _, port := range service.Spec.Ports {
			log.Printf("Service port: %d, NodePort: %d, TargetPort: %v", port.Port, port.NodePort, port.TargetPort)
			if port.Port == 5000 || port.TargetPort.IntVal == 5000 {
				if port.NodePort == 0 {
					return fmt.Errorf("webui service port 5000 does not have a NodePort")
				}
				w.webuiPort = port.NodePort
				log.Printf("WebUI NodePort: %d", w.webuiPort)
				return nil
			}
		}
	}

	// If not found, list all services to help debug
	log.Println("Could not find webui service. Listing all services:")
	for _, namespace := range namespaces {
		services, err := w.clientset.CoreV1().Services(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			log.Printf("Failed to list services in namespace %s: %v", namespace, err)
			continue
		}

		for _, service := range services.Items {
			log.Printf("Namespace: %s, Service: %s, Ports: %v", namespace, service.Name, service.Spec.Ports)
		}
	}

	return fmt.Errorf("webui service not found in any namespace")
}

func (w *WebUIAutomation) automateBrowser() error {
	var iteration int
	iteration = 100
	// Create context for Chrome automation
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false), // Run in visible mode
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// Set timeout for the entire automation (increased for 9 iterations)
	ctx, cancel = context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	log.Printf("Opening Chrome and navigating to: %s", w.webuiURL)

	// Login and initial navigation
	err := chromedp.Run(ctx,
		// Navigate to the webui base URL for login
		chromedp.Navigate(w.webuiURL),

		// Wait for page to be fully loaded
		w.waitForPageLoad(),

		// Handle login if login form is present
		w.handleLogin(),

		// Wait after login
		chromedp.Sleep(2*time.Second),
	)

	if err != nil {
		return fmt.Errorf("failed during login: %w", err)
	}
	w.imsiCounter = 3518 // Initialize IMSI counter
	total := w.imsiCounter + iteration
	// Repeat the subscriber creation process times
	for i := 1; i <= iteration; i++ {
		log.Printf("Starting subscriber creation iteration %d/%d", i, total)

		err := chromedp.Run(ctx,
			// Navigate to subscriber page
			chromedp.Navigate(w.webuiURL+"/subscriber/create"),

			// Wait for subscriber page to be fully loaded
			w.waitForPageLoad(),

			// Modify IMSI number (increment by 1)
			w.modifyIMSI(),

			// Change Operator code type from OPc to OP
			w.changeOperatorCodeType(),

			// Click create/submit button
			w.submitForm(),

			// Wait to see the result
			chromedp.Sleep(3*time.Second),
		)

		if err != nil {
			log.Printf("Error during iteration %d: %v", i, err)
			// Continue with next iteration instead of failing completely
			continue
		}

		log.Printf("Completed subscriber creation iteration %d/%d", i, iteration)
		w.imsiCounter++
		// Small delay between iterations
		chromedp.Sleep(1 * time.Second).Do(ctx)
	}

	log.Printf("Completed all %d subscriber creation iterations", iteration)
	return nil
}

func (w *WebUIAutomation) handleLogin() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		log.Println("Checking for login form...")

		// Check if login form is present by looking for username/password fields
		var loginFormExists bool
		err := chromedp.Evaluate(`
			document.querySelector('input[name*="username"], input[name*="user"], input[type="text"], input[id*="username"], input[id*="user"]') !== null &&
			document.querySelector('input[type="password"], input[name*="password"], input[id*="password"]') !== null
		`, &loginFormExists).Do(ctx)

		if err != nil || !loginFormExists {
			log.Println("No login form detected, proceeding...")
			return nil
		}

		log.Println("Login form detected, attempting to login with admin/free5gc...")

		// Try different selectors for username field
		usernameSelectors := []string{
			`input[name*="username"]`,
			`input[name*="user"]`,
			`input[id*="username"]`,
			`input[id*="user"]`,
			`input[placeholder*="username"]`,
			`input[placeholder*="user"]`,
			`input[type="text"]`,
		}

		var usernameSelector string
		for _, selector := range usernameSelectors {
			var exists bool
			chromedp.Evaluate(fmt.Sprintf(`document.querySelector('%s') !== null`, selector), &exists).Do(ctx)
			if exists {
				usernameSelector = selector
				log.Printf("Found username field with selector: %s", selector)
				break
			}
		}

		// Try different selectors for password field
		passwordSelectors := []string{
			`input[type="password"]`,
			`input[name*="password"]`,
			`input[id*="password"]`,
			`input[placeholder*="password"]`,
		}

		var passwordSelector string
		for _, selector := range passwordSelectors {
			var exists bool
			chromedp.Evaluate(fmt.Sprintf(`document.querySelector('%s') !== null`, selector), &exists).Do(ctx)
			if exists {
				passwordSelector = selector
				log.Printf("Found password field with selector: %s", selector)
				break
			}
		}

		if usernameSelector == "" || passwordSelector == "" {
			log.Println("Could not find username or password fields")
			return nil
		}

		// Fill in the login form
		err = chromedp.Run(ctx,
			chromedp.Clear(usernameSelector, chromedp.ByQuery),
			chromedp.SendKeys(usernameSelector, "admin", chromedp.ByQuery),
			chromedp.Clear(passwordSelector, chromedp.ByQuery),
			chromedp.SendKeys(passwordSelector, "free5gc", chromedp.ByQuery),
		)

		if err != nil {
			log.Printf("Failed to fill login form: %v", err)
			return nil
		}

		// Try to find and click login button
		loginButtonSelectors := []string{
			`button[type="submit"]`,
			`input[type="submit"]`,
			`button[contains(text(), "Login")]`,
			`button[contains(text(), "Sign")]`,
			`button[contains(text(), "Submit")]`,
			`.btn-primary`,
			`button[class*="login"]`,
			`button[id*="login"]`,
			`form button`,
		}

		for _, selector := range loginButtonSelectors {
			log.Printf("Trying to click login button with selector: %s", selector)
			err := chromedp.Click(selector, chromedp.ByQuery).Do(ctx)
			if err == nil {
				log.Println("Successfully clicked login button")
				return nil
			}
		}

		// If no button found, try pressing Enter on password field
		log.Println("No login button found, trying Enter key on password field")
		return chromedp.SendKeys(passwordSelector, "\n", chromedp.ByQuery).Do(ctx)
	})
}

func (w *WebUIAutomation) clickCreateButton() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		// Try multiple selectors for the create button
		selectors := []string{
			`button[contains(text(), ""CREATE"")]`,
			`input[value="CREATE"]`,
			`button[class*="CREATE"]`,
			`button[id*="CREATE"]`,
			`.btn-primary`,
			`button[type="button"]`,
			`a[href*="create"]`,
			`button:contains(""CREATE"")`,
		}

		for _, selector := range selectors {
			log.Printf("Trying to click create button with selector: %s", selector)
			err := chromedp.Click(selector, chromedp.ByQuery).Do(ctx)
			if err == nil {
				log.Println("Successfully clicked create button")
				return nil
			}
		}

		// If no button found, try to find any clickable elements and log them
		var elements []string
		chromedp.Evaluate(`
			Array.from(document.querySelectorAll('button, input[type="submit"], input[type="button"], a')).map(el => ({
				tag: el.tagName,
				text: el.textContent.trim(),
				value: el.value || '',
				class: el.className || '',
				id: el.id || ''
			}))
		`, &elements).Do(ctx)

		log.Printf("Available clickable elements: %v", elements)
		return fmt.Errorf("could not find create button")
	})
}

func (w *WebUIAutomation) modifyIMSI() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		log.Println("Looking for IMSI input field...")

		// Try to find IMSI input field with more specific selectors
		selectors := []string{
			`input[id=":r3:"]`,                      // Exact ID from HTML
			`input[name="ueId"]`,                    // Name attribute
			`input[name*="ueId"]`,                   // Partial name match
			`input[id*=":r3:"]`,                     // Partial ID match
			`input[placeholder*="IMSI"]`,            // Placeholder text
			`input[placeholder*="imsi"]`,            // Placeholder text (lowercase)
			`input.MuiInputBase-input[type="text"]`, // MUI input class
		}

		for _, selector := range selectors {
			log.Printf("Trying to find IMSI field with selector: %s", selector)

			// Check if element exists first
			var exists bool
			err := chromedp.Evaluate(fmt.Sprintf(`document.querySelector('%s') !== null`, selector), &exists).Do(ctx)
			if err != nil || !exists {
				log.Printf("Element not found with selector: %s", selector)
				continue
			}

			var currentValue string
			err = chromedp.Value(selector, &currentValue, chromedp.ByQuery).Do(ctx)
			if err != nil {
				log.Printf("Failed to get value from selector %s: %v", selector, err)
				continue
			}

			log.Printf("Found IMSI field with current value: '%s'", currentValue)

			// If field is empty, provide a default IMSI
			if currentValue == "" {
				defaultIMSI := "imsi-208930000000001"
				log.Printf("IMSI field is empty, using default: %s", defaultIMSI)

				// Clear field and set default value using JavaScript for reliable clearing
				err = chromedp.Run(ctx,
					chromedp.Focus(selector, chromedp.ByQuery),
					chromedp.Evaluate(fmt.Sprintf(`document.querySelector('%s').value = ''`, selector), nil),
					chromedp.SendKeys(selector, defaultIMSI, chromedp.ByQuery),
				)
				if err != nil {
					log.Printf("Failed to set default IMSI: %v", err)
					continue
				}
				log.Printf("Successfully set default IMSI: %s", defaultIMSI)
				return nil
			}

			// Parse current IMSI and increment by 1
			if len(currentValue) >= 15 {
				// IMSI is typically 15 digits - extract the last digit and increment
				lastDigitStr := currentValue[len(currentValue)-1:]
				if lastDigit, parseErr := strconv.Atoi(lastDigitStr); parseErr == nil {
					// Increment the last digit, wrap around if needed
					newLastDigit := lastDigit + w.imsiCounter
					newIMSI := currentValue[:len(currentValue)-4] + strconv.Itoa(newLastDigit)
					// newIMSI := currentValue[:len(currentValue)-1] + strconv.Itoa(newLastDigit)
					// newIMSI := currentValue[:len(currentValue)-2] + strconv.Itoa(newLastDigit)
					// newIMSI := currentValue[:len(currentValue)-3] + strconv.Itoa(newLastDigit)

					log.Printf("Changing IMSI from %s to %s", currentValue, newIMSI)

					// Clear field completely using JavaScript and then set new value
					err = chromedp.Run(ctx,
						chromedp.Focus(selector, chromedp.ByQuery),
						chromedp.Sleep(100*time.Millisecond),
						// Use JavaScript to clear the field completely
						chromedp.Evaluate(fmt.Sprintf(`
							const element = document.querySelector('%s');
							element.value = '';
							element.dispatchEvent(new Event('input', { bubbles: true }));
							element.dispatchEvent(new Event('change', { bubbles: true }));
						`, selector), nil),
						chromedp.Sleep(100*time.Millisecond),
						chromedp.SendKeys(selector, newIMSI, chromedp.ByQuery),
					)
					if err != nil {
						log.Printf("Failed to update IMSI: %v", err)
						continue
					}

					// Verify the value was set correctly
					var verifyValue string
					chromedp.Value(selector, &verifyValue, chromedp.ByQuery).Do(ctx)
					log.Printf("IMSI value after update: %s", verifyValue)

					return nil
				} else {
					log.Printf("Failed to parse last digit of IMSI: %v", parseErr)
				}
			} else {
				log.Printf("IMSI too short (length: %d), expected at least 15 digits", len(currentValue))

				// If IMSI is too short, try to pad it or use default
				if len(currentValue) > 0 {
					// Pad with zeros to make it 15 digits
					paddedIMSI := currentValue + strings.Repeat("0", 15-len(currentValue))
					log.Printf("Padding short IMSI to: %s", paddedIMSI)

					err = chromedp.Run(ctx,
						chromedp.Focus(selector, chromedp.ByQuery),
						chromedp.Evaluate(fmt.Sprintf(`document.querySelector('%s').value = ''`, selector), nil),
						chromedp.SendKeys(selector, paddedIMSI, chromedp.ByQuery),
					)
					if err == nil {
						return nil
					}
				}
			}
		}

		log.Println("Could not find or modify IMSI field")
		return nil // Don't fail the entire automation
	})
}

func (w *WebUIAutomation) changeOperatorCodeType() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		log.Println("Looking for operator code type field...")

		// Try to find the MUI select component for operator code
		selectors := []string{
			`div[id="mui-component-select-auth.operatorCodeType"]`,
			`div[aria-labelledby="mui-component-select-auth.operatorCodeType"]`,
			`div[role="combobox"][class*="MuiSelect"]`,
			`div[role="combobox"]:contains("OPc")`,
			`div.MuiSelect-select:contains("OPc")`,
		}

		for _, selector := range selectors {
			log.Printf("Trying to find operator code field with selector: %s", selector)

			// Check if the element exists
			var exists bool
			err := chromedp.Evaluate(fmt.Sprintf(`document.querySelector('%s') !== null`, selector), &exists).Do(ctx)
			if err != nil || !exists {
				log.Printf("Element not found with selector: %s", selector)
				continue
			}

			log.Printf("Found operator code field with selector: %s", selector)

			// Click on the MUI select to open dropdown
			err = chromedp.Click(selector, chromedp.ByQuery).Do(ctx)
			if err != nil {
				log.Printf("Failed to click operator code field: %v", err)
				continue
			}

			log.Println("Clicked operator code field, waiting for dropdown...")
			chromedp.Sleep(500 * time.Millisecond).Do(ctx)

			// Try to find and click the "OP" option in the dropdown
			optionSelectors := []string{
				`li[data-value="OP"]`,
				`li:contains("OP")`,
				`div[role="option"]:contains("OP")`,
				`ul[role="listbox"] li:contains("OP")`,
				`.MuiMenuItem-root:contains("OP")`,
			}

			for _, optionSelector := range optionSelectors {
				log.Printf("Trying to click OP option with selector: %s", optionSelector)
				err = chromedp.Click(optionSelector, chromedp.ByQuery).Do(ctx)
				if err == nil {
					log.Println("Successfully changed operator code type to OP")
					return nil
				}
			}

			// If specific "OP" option not found, try to find all options and log them
			var options []map[string]interface{}
			chromedp.Evaluate(`
				Array.from(document.querySelectorAll('li[role="option"], .MuiMenuItem-root, ul[role="listbox"] li')).map(el => ({
					text: el.textContent.trim(),
					value: el.getAttribute('data-value') || '',
					tagName: el.tagName
				}))
			`, &options).Do(ctx)

			log.Printf("Available dropdown options: %v", options)

			// Try clicking any option that contains "OP" (case insensitive)
			for _, option := range options {
				if text, ok := option["text"].(string); ok {
					if strings.ToUpper(strings.TrimSpace(text)) == "OP" {
						// Found exact match, try to click it
						err = chromedp.Evaluate(fmt.Sprintf(`
							Array.from(document.querySelectorAll('li[role="option"], .MuiMenuItem-root, ul[role="listbox"] li'))
							.find(el => el.textContent.trim().toUpperCase() === 'OP').click()
						`), nil).Do(ctx)
						if err == nil {
							log.Println("Successfully clicked OP option via JavaScript")
							return nil
						}
					}
				}
			}

			break // Exit the selector loop since we found the field but couldn't select the option
		}

		log.Println("Could not find or change operator code type field")
		return nil // Don't fail the entire automation
	})
}

func (w *WebUIAutomation) submitForm() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		log.Println("Looking for submit/create button...")

		// Try multiple selectors for the MUI submit button, including the exact structure from the HTML
		selectors := []string{
			`button.MuiButtonBase-root.MuiButton-root.MuiButton-contained.MuiButton-containedPrimary[type="button"]`, // Exact class match with type="button"
			`button.css-1128369`, // Specific CSS class from the HTML
			`button[class*="MuiButton-containedPrimary"][type="button"]`,                       // MUI primary button with button type
			`button[class*="MuiButtonBase-root"][class*="MuiButton-contained"][type="button"]`, // Combined classes
			`button[tabindex="0"][type="button"]`,                                              // Button with tabindex and button type
			`button.MuiButton-root[type="button"]`,                                             // MUI button root with button type
			`button[type="button"]`,                                                            // Any button type="button"
		}

		// First, wait for the button to be present and visible
		log.Println("Waiting for CREATE button to be visible...")
		chromedp.Sleep(2 * time.Second).Do(ctx)

		for _, selector := range selectors {
			log.Printf("Trying to click submit button with selector: %s", selector)

			// Check if element exists and is visible
			var exists bool
			err := chromedp.Evaluate(fmt.Sprintf(`
				const element = document.querySelector('%s');
				element !== null && element.offsetParent !== null && element.textContent.trim().includes('CREATE')
			`, selector), &exists).Do(ctx)

			if err != nil || !exists {
				log.Printf("Element not found or not visible with selector: %s", selector)
				continue
			}

			log.Printf("Found submit button with selector: %s", selector)

			// Wait for the element to be ready and try to click
			err = chromedp.Run(ctx,
				chromedp.WaitVisible(selector, chromedp.ByQuery),
				chromedp.Sleep(500*time.Millisecond), // Small delay to ensure element is ready
				chromedp.Click(selector, chromedp.ByQuery),
			)

			if err == nil {
				log.Println("Successfully clicked submit button")
				return nil
			} else {
				log.Printf("Failed to click button with selector %s: %v", selector, err)
			}
		}

		// If direct clicking failed, try JavaScript click with the exact button structure
		log.Println("Direct clicking failed, trying JavaScript click...")
		var jsClickSuccess bool
		err := chromedp.Evaluate(`
			// Find button by exact class match and text content
			const button = Array.from(document.querySelectorAll('button')).find(btn => 
				btn.classList.contains('MuiButtonBase-root') &&
				btn.classList.contains('MuiButton-contained') &&
				btn.classList.contains('MuiButton-containedPrimary') &&
				btn.type === 'button' &&
				btn.textContent.trim().includes('CREATE')
			);
			
			if (button) {
				console.log('Found CREATE button via JavaScript, attempting click...');
				button.click();
				true;
			} else {
				console.log('CREATE button not found via JavaScript');
				false;
			}
		`, &jsClickSuccess).Do(ctx)

		if err == nil && jsClickSuccess {
			log.Println("Successfully clicked CREATE button via JavaScript")
			return nil
		}

		// Enhanced fallback: try to find all buttons and log detailed information
		log.Println("JavaScript click failed, searching for all buttons...")
		var buttons []map[string]interface{}
		chromedp.Evaluate(`
			Array.from(document.querySelectorAll('button')).map(el => ({
				tag: el.tagName,
				text: el.textContent.trim(),
				type: el.type || '',
				class: el.className || '',
				id: el.id || '',
				tabindex: el.tabIndex || '',
				disabled: el.disabled,
				visible: el.offsetParent !== null,
				hasCreateText: el.textContent.trim().includes('CREATE')
			}))
		`, &buttons).Do(ctx)

		log.Printf("Available buttons: %v", buttons)

		// Final fallback: try to trigger click event directly on CREATE button
		log.Println("Trying direct event dispatch on CREATE button...")
		err = chromedp.Evaluate(`
			const createButtons = Array.from(document.querySelectorAll('button')).filter(btn => 
				btn.textContent.trim().includes('CREATE')
			);
			
			if (createButtons.length > 0) {
				const button = createButtons[0];
				console.log('Dispatching click event on CREATE button...');
				
				// Dispatch multiple events to ensure it's processed
				button.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
				button.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }));
				button.dispatchEvent(new MouseEvent('click', { bubbles: true }));
				
				// Also try focus and enter key
				button.focus();
				button.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
				
				true;
			} else {
				false;
			}
		`, nil).Do(ctx)

		if err == nil {
			log.Println("Successfully dispatched click events on CREATE button")
			return nil
		}

		log.Println("Could not find or click submit button")
		return nil // Don't fail the entire automation
	})
}

// Helper function to wait for page to be fully loaded
func (w *WebUIAutomation) waitForPageLoad() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		log.Println("Waiting for page to be fully loaded...")

		// Wait for document ready state to be complete
		var readyState string
		for i := 0; i < 30; i++ { // Wait up to 30 seconds
			err := chromedp.Evaluate(`document.readyState`, &readyState).Do(ctx)
			if err != nil {
				return err
			}
			log.Printf("Document ready state: %s", readyState)
			if readyState == "complete" {
				break
			}
			time.Sleep(1 * time.Second)
		}

		// Additional wait for any dynamic content loading
		return chromedp.Sleep(1 * time.Second).Do(ctx)
	})
}

// Helper function to wait for specific elements to be present and visible
func (w *WebUIAutomation) waitForElementsReady(selectors []string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		log.Println("Waiting for page elements to be ready...")

		// Wait for at least one of the selectors to be visible
		for _, selector := range selectors {
			err := chromedp.WaitVisible(selector, chromedp.ByQuery).Do(ctx)
			if err == nil {
				log.Printf("Element ready: %s", selector)
				return nil
			}
		}

		log.Println("No expected elements found, but continuing...")
		return nil
	})
}
