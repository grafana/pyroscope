/*
Ginkgo is usually dot-imported via:

	import . "github.com/onsi/ginkgo/v2"

however some parts of the DSL may conflict with existing symbols in the user's code.

To mitigate this without losing the brevity of dot-importing Ginkgo the various packages in the
dsl directory provide pieces of the Ginkgo DSL that can be dot-imported separately.

This "core" package pulls in the core Ginkgo DSL - most test suites will only need to import this package.
*/
package core

import (
	"github.com/onsi/ginkgo/v2"
)

const GINKGO_VERSION = ginkgo.GINKGO_VERSION

type GinkgoWriterInterface = ginkgo.GinkgoWriterInterface
type GinkgoTestingT = ginkgo.GinkgoTestingT
type GinkgoTInterface = ginkgo.GinkgoTInterface
type FullGinkgoTInterface = ginkgo.FullGinkgoTInterface
type SpecContext = ginkgo.SpecContext

var GinkgoWriter = ginkgo.GinkgoWriter
var GinkgoLogr = ginkgo.GinkgoLogr
var GinkgoConfiguration = ginkgo.GinkgoConfiguration
var GinkgoRandomSeed = ginkgo.GinkgoRandomSeed
var GinkgoParallelProcess = ginkgo.GinkgoParallelProcess
var GinkgoHelper = ginkgo.GinkgoHelper
var GinkgoLabelFilter = ginkgo.GinkgoLabelFilter
var PauseOutputInterception = ginkgo.PauseOutputInterception
var ResumeOutputInterception = ginkgo.ResumeOutputInterception
var RunSpecs = ginkgo.RunSpecs
var Skip = ginkgo.Skip
var Fail = ginkgo.Fail
var AbortSuite = ginkgo.AbortSuite
var GinkgoRecover = ginkgo.GinkgoRecover
var Describe = ginkgo.Describe
var FDescribe = ginkgo.FDescribe
var PDescribe = ginkgo.PDescribe
var XDescribe = PDescribe
var Context, FContext, PContext, XContext = Describe, FDescribe, PDescribe, XDescribe
var When, FWhen, PWhen, XWhen = Describe, FDescribe, PDescribe, XDescribe
var It = ginkgo.It
var FIt = ginkgo.FIt
var PIt = ginkgo.PIt
var XIt = PIt
var Specify, FSpecify, PSpecify, XSpecify = It, FIt, PIt, XIt
var By = ginkgo.By
var BeforeSuite = ginkgo.BeforeSuite
var AfterSuite = ginkgo.AfterSuite
var SynchronizedBeforeSuite = ginkgo.SynchronizedBeforeSuite
var SynchronizedAfterSuite = ginkgo.SynchronizedAfterSuite
var BeforeEach = ginkgo.BeforeEach
var JustBeforeEach = ginkgo.JustBeforeEach
var AfterEach = ginkgo.AfterEach
var JustAfterEach = ginkgo.JustAfterEach
var BeforeAll = ginkgo.BeforeAll
var AfterAll = ginkgo.AfterAll
var DeferCleanup = ginkgo.DeferCleanup
var GinkgoT = ginkgo.GinkgoT
var AttachProgressReporter = ginkgo.AttachProgressReporter
