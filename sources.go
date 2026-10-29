package tap

// Payment source IDs for the source.id field of charge and authorize
// requests. A token ID (tok_...) or an authorize ID (auth_...) can be used
// as the source instead, to charge a tokenized card or capture a hold.
//
// See https://developers.tap.company/reference/charges.
const (
	// SourceAll shows every payment method enabled for the merchant on Tap's
	// hosted page.
	SourceAll = "src_all"
	// SourceCard shows only card payment methods on Tap's hosted page.
	SourceCard = "src_card"

	// SourceMada is mada, the local payment switch in Saudi Arabia.
	SourceMada = "src_sa.mada"
	// SourceKNET is KNET, the local payment switch in Kuwait.
	SourceKNET = "src_kw.knet"
	// SourceBenefit is Benefit, the local payment switch in Bahrain.
	SourceBenefit = "src_bh.benefit"
	// SourceBenefitPay is the BenefitPay wallet in Bahrain.
	SourceBenefitPay = "src_benefitpay"
	// SourceQPay is NAPS/QPAY, the local payment switch in Qatar.
	SourceQPay = "src_qa.qpay"
	// SourceOmanNet is OmanNet, the local payment switch in Oman.
	SourceOmanNet = "src_omannet"
	// SourceFawry is Fawry, the local payment switch in Egypt.
	SourceFawry = "src_eg.fawry"
	// SourceSTCPay is STC Pay, paid with a phone number registered in STC Pay
	// in Saudi Arabia.
	SourceSTCPay = "src_sa.stcpay"

	// SourceApplePay is Apple Pay.
	SourceApplePay = "src_apple_pay"
	// SourceGooglePay is Google Pay.
	SourceGooglePay = "src_google_pay"
	// SourceSamsungPay is Samsung Pay.
	SourceSamsungPay = "src_samsung_pay"

	// SourceTabby is Tabby installments. The misspelling is Tap's actual ID.
	SourceTabby = "src_tabby.installement"
	// SourceTamara is Tamara buy now, pay later.
	SourceTamara = "src_tamara"
	// SourceDeema is Deema buy now, pay later.
	SourceDeema = "src_deema"
)
