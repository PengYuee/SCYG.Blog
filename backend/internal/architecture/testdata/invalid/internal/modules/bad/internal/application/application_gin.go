package application

import "github.com/gin-gonic/gin"

// Port is an application fixture.
type Port interface{ Run(*gin.Context) }
