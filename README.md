pagsego
=======

PagSeguro API in Go (golang) (WIP)

    req := pagsego.NewPaymentRequest("TOKEN", "vendedor@email.com", "REFID", "REDIRECTURI", "NOTIFICATIONURI")
    req.AddItem("ID", "DESCRIPTION", 23.56, 1)
    req.SetBuyer("Nome do Comprador", "comprador@email.com").SetCPF("00000000000")
    req.SetShipping(pagsego.ShippingSEDEX, 10.0).SetAddress("SP", "São Paulo", "00000000", "Bairro", "Rua Teste", "1040", "Apt 111")

    fmt.Println(result.Success)
    if result.Success {
    	fmt.Println(result.CheckoutResponse.Code)
    }
    