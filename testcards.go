package tap

// Tap sandbox test cards, from
// https://developers.tap.company/reference/testing-cards. These only work
// with a test secret key (sk_test_...).
//
// For credit and debit cards, the expiry date picks the outcome (see the
// TestExpiry values) and the CVV picks the CVV check result (see the
// TestCVV constants).
const (
	TestCardMastercard3DS = "5123450000000008"
	TestCardMastercard    = "5111111111111118"
	TestCardVisa3DS       = "4508750015741019"
	TestCardVisa          = "4012000033330026"
	TestCardAmex3DS       = "345678901234564"
	TestCardAmex          = "371449635398431"
	TestCardMada3DS       = "4464040000000007"
	TestCardMada          = "5588480000000003"
	TestCardOmanNet3DS    = "4837915060379278"

	TestCardKNETCaptured    = "8888880000000001"
	TestCardBenefitCaptured = "4600410123456789"
	TestCardBenefitDeclined = "1111110123456789"
	TestCardQPayCaptured    = "4215375500883243"
	TestCardJaywanCaptured  = "6690109000011016"
	TestCardJaywanDeclined  = "6690109000011032"
)

// CVV values that select the sandbox CVV check result for credit and debit
// cards. American Express uses four digits.
const (
	TestCVVMatch            = "100"
	TestCVVNotProcessed     = "101"
	TestCVVNoMatch          = "102"
	TestCVVAmexMatch        = "1000"
	TestCVVAmexNotProcessed = "1010"
	TestCVVAmexNoMatch      = "1020"
)

// TestExpiry is a sandbox expiry date that selects a transaction outcome for
// credit and debit cards. Month and Year are two digits, as the card token
// request expects.
type TestExpiry struct {
	Month  int
	Year   int
	Result string
}

// Sandbox expiry dates and the outcome each one produces.
var (
	TestExpiryApproved = TestExpiry{
		Month:  1,
		Year:   39,
		Result: "APPROVED",
	}
	TestExpiryDeclined = TestExpiry{
		Month:  5,
		Year:   22,
		Result: "DECLINED",
	}
	TestExpiryExpiredCard = TestExpiry{
		Month:  4,
		Year:   27,
		Result: "EXPIRED_CARD",
	}
	TestExpiryTimedOut = TestExpiry{
		Month:  8,
		Year:   28,
		Result: "TIMED_OUT",
	}
	TestExpiryAcquirerSystemError = TestExpiry{
		Month:  1,
		Year:   37,
		Result: "ACQUIRER_SYSTEM_ERROR",
	}
	TestExpiryUnspecifiedFailure = TestExpiry{
		Month:  2,
		Year:   37,
		Result: "UNSPECIFIED_FAILURE",
	}
	TestExpiryUnknown = TestExpiry{
		Month:  5,
		Year:   37,
		Result: "UNKNOWN",
	}
	TestExpiryOmanNetApproved = TestExpiry{
		Month:  7,
		Year:   30,
		Result: "APPROVED",
	}
)

// TestCard is a Tap sandbox card.
type TestCard struct {
	// Number is the card number.
	Number string
	// Method is the payment method, e.g. VISA, MADA, KNET, BENEFIT.
	Method string
	// ThreeDSecure reports whether the card is enrolled in 3D Secure. It
	// applies to credit and debit cards.
	ThreeDSecure bool
	// ExpMonth and ExpYear (two digits) are the documented expiry for local
	// payment method cards. Credit and debit cards take a TestExpiry instead.
	ExpMonth int
	ExpYear  int
	// PIN is the documented PIN or CVV for local payment method cards. Some
	// also need an OTP, noted in Note.
	PIN string
	// ExpectedStatus is the documented result for local payment method
	// cards: CAPTURED, NOT CAPTURED, or DECLINED.
	ExpectedStatus string
	// Note holds extra instructions from the docs.
	Note string
}

// TestCards lists every card in Tap's sandbox test card table.
var TestCards = []TestCard{
	{Number: TestCardMastercard3DS, Method: "MASTERCARD", ThreeDSecure: true},
	{Number: TestCardMastercard, Method: "MASTERCARD"},
	{Number: TestCardVisa3DS, Method: "VISA", ThreeDSecure: true},
	{Number: TestCardVisa, Method: "VISA"},
	{Number: "4440000009900010", Method: "VISA", ThreeDSecure: true},
	{Number: "4440000042200014", Method: "VISA", ThreeDSecure: true},
	{Number: "4440000042200022", Method: "VISA", ThreeDSecure: true},
	{Number: TestCardAmex3DS, Method: "AMEX", ThreeDSecure: true},
	{Number: TestCardAmex, Method: "AMEX"},
	{Number: TestCardMada3DS, Method: "MADA", ThreeDSecure: true},
	{Number: TestCardMada, Method: "MADA"},
	{
		Number:       TestCardOmanNet3DS,
		Method:       "OMANNET",
		ThreeDSecure: true,
		Note:         "CVV 844, OTP 9999; expiry 07/30 approves",
	},

	{
		Number:         TestCardKNETCaptured,
		Method:         "KNET",
		ExpMonth:       9,
		ExpYear:        30,
		PIN:            "1234",
		ExpectedStatus: "CAPTURED",
		Note:           "select KNET Test Card [KNET1] as the bank",
	},
	{
		Number:         "8888880000000002",
		Method:         "KNET",
		ExpMonth:       9,
		ExpYear:        30,
		PIN:            "1234",
		ExpectedStatus: "CAPTURED",
		Note:           "select KNET Test Card [KNET1] as the bank",
	},
	{
		Number:         TestCardKNETCaptured,
		Method:         "KNET",
		ExpMonth:       5,
		ExpYear:        21,
		PIN:            "1234",
		ExpectedStatus: "NOT CAPTURED",
		Note:           "select KNET Test Card [KNET1] as the bank",
	},
	{
		Number:         TestCardBenefitCaptured,
		Method:         "BENEFIT",
		ExpMonth:       12,
		ExpYear:        27,
		PIN:            "1234",
		ExpectedStatus: "CAPTURED",
	},
	{
		Number:         "7777770123456789",
		Method:         "BENEFIT",
		ExpMonth:       12,
		ExpYear:        27,
		PIN:            "1234",
		ExpectedStatus: "NOT CAPTURED",
	},
	{
		Number:         TestCardBenefitDeclined,
		Method:         "BENEFIT",
		ExpMonth:       12,
		ExpYear:        27,
		PIN:            "1234",
		ExpectedStatus: "DECLINED",
	},
	{
		Number:         TestCardQPayCaptured,
		Method:         "NAPS/QPAY",
		ExpMonth:       12,
		ExpYear:        26,
		PIN:            "944",
		ExpectedStatus: "CAPTURED",
		Note:           "OTP 1234",
	},
	{
		Number:         "4712580028965277",
		Method:         "NAPS/QPAY",
		ExpMonth:       12,
		ExpYear:        26,
		PIN:            "878",
		ExpectedStatus: "CAPTURED",
		Note:           "OTP 1234",
	},
	{
		Number:         "5318012475972742",
		Method:         "NAPS/QPAY",
		ExpMonth:       10,
		ExpYear:        26,
		PIN:            "944",
		ExpectedStatus: "CAPTURED",
		Note:           "OTP 1234",
	},
	{
		Number:         TestCardJaywanCaptured,
		Method:         "JAYWAN",
		ExpMonth:       1,
		ExpYear:        39,
		PIN:            "100",
		ExpectedStatus: "CAPTURED",
	},
	{
		Number:         "6690109000011008",
		Method:         "JAYWAN",
		ExpMonth:       1,
		ExpYear:        39,
		PIN:            "100",
		ExpectedStatus: "CAPTURED",
	},
	{
		Number:         TestCardJaywanDeclined,
		Method:         "JAYWAN",
		ExpMonth:       1,
		ExpYear:        39,
		PIN:            "100",
		ExpectedStatus: "DECLINED",
	},
	{
		Number:         "6690109000011057",
		Method:         "JAYWAN",
		ExpMonth:       1,
		ExpYear:        39,
		PIN:            "100",
		ExpectedStatus: "DECLINED",
	},
}

// TestSTCPayPhones are sandbox STC Pay phone numbers (country code 966).
var TestSTCPayPhones = []string{
	"548220713",
	"550955806",
	"554748162",
	"554774102",
}
